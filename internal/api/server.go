package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	core "github.com/iden3/go-iden3-core"

	"github.com/polygonid/sh-id-platform/internal/config"
	"github.com/polygonid/sh-id-platform/internal/core/ports"
	"github.com/polygonid/sh-id-platform/internal/core/services"
	"github.com/polygonid/sh-id-platform/internal/repositories"
)

// Server implements StrictServerInterface and holds the implementation of all API controllers
// This is the glue to the API autogenerated code
type Server struct {
	cfg             *config.Configuration
	identityService ports.IndentityService
	claimService    ports.ClaimsService
	schemaService   ports.SchemaService
}

// NewServer is a Server constructor
func NewServer(cfg *config.Configuration, identityService ports.IndentityService, claimsService ports.ClaimsService, schemaService ports.SchemaService) *Server {
	return &Server{
		cfg:             cfg,
		identityService: identityService,
		claimService:    claimsService,
		schemaService:   schemaService,
	}
}

// Health is a method
func (s *Server) Health(_ context.Context, _ HealthRequestObject) (HealthResponseObject, error) {
	return Health200JSONResponse{
		Cache: true,
		Db:    false,
	}, nil
}

// GetDocumentation this method will be overridden in the main function
func (s *Server) GetDocumentation(_ context.Context, _ GetDocumentationRequestObject) (GetDocumentationResponseObject, error) {
	return nil, nil
}

// GetYaml this method will be overridden in the main function
func (s *Server) GetYaml(_ context.Context, _ GetYamlRequestObject) (GetYamlResponseObject, error) {
	return nil, nil
}

// RegisterStatic add method to the mux that are not documented in the API.
func RegisterStatic(mux *chi.Mux) {
	mux.Get("/", documentation)
	mux.Get("/static/docs/api/api.yaml", swagger)
}

func documentation(w http.ResponseWriter, _ *http.Request) {
	writeFile("api/spec.html", w)
}

func swagger(w http.ResponseWriter, _ *http.Request) {
	writeFile("api/api.yaml", w)
}

func writeFile(path string, w http.ResponseWriter) {
	f, err := os.ReadFile(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f)
}

// CreateIdentity is created identity controller
func (s *Server) CreateIdentity(ctx context.Context, request CreateIdentityRequestObject) (CreateIdentityResponseObject, error) {
	identity, err := s.identityService.Create(ctx, fmt.Sprintf("%s:%d", s.cfg.ServerUrl, s.cfg.ServerPort))
	if err != nil {
		return nil, err
	}
	return CreateIdentity201JSONResponse{
		Identifier: &identity.Identifier,
		Immutable:  identity.Immutable,
		Relay:      identity.Relay,
		State: &IdentityState{
			BlockNumber:        identity.State.BlockNumber,
			BlockTimestamp:     identity.State.BlockTimestamp,
			ClaimsTreeRoot:     identity.State.ClaimsTreeRoot,
			CreatedAt:          identity.State.CreatedAt,
			ModifiedAt:         identity.State.ModifiedAt,
			PreviousState:      identity.State.PreviousState,
			RevocationTreeRoot: identity.State.RevocationTreeRoot,
			RootOfRoots:        identity.State.RootOfRoots,
			State:              identity.State.State,
			Status:             string(identity.State.Status),
			TxID:               identity.State.TxID,
		},
	}, nil
}

// CreateClaim is claim creation controller
func (s *Server) CreateClaim(ctx context.Context, request CreateClaimRequestObject) (CreateClaimResponseObject, error) {
	did, err := core.ParseDID(request.Identifier)
	if err != nil {
		return CreateClaim400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
	}

	claimReq := ports.NewClaimRequest(request.Body.CredentialSchema, did, request.Body.CredentialSchema, request.Body.CredentialSubject, request.Body.Expiration, request.Body.Type, request.Body.Version, request.Body.SubjectPosition, request.Body.MerklizedRootPosition)

	resp, err := s.claimService.CreateClaim(ctx, claimReq)
	if err != nil {
		if errors.Is(err, services.ErrJSONLdContext) {
			return CreateClaim400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrProcessSchema) {
			return CreateClaim400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		return CreateClaim500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return CreateClaim201JSONResponse{Id: resp.ID.String()}, nil
}

// RevokeClaim is the revocation claim controller
func (s *Server) RevokeClaim(ctx context.Context, request RevokeClaimRequestObject) (RevokeClaimResponseObject, error) {
	if err := s.claimService.Revoke(ctx, request.Identifier, uint64(request.Nonce), ""); err != nil {
		if errors.Is(err, repositories.ErrClaimDoesNotExist) {
			return RevokeClaim404JSONResponse{N404JSONResponse{
				Message: "the claim does not exist",
			}}, nil
		}

		return RevokeClaim500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return RevokeClaim202JSONResponse{
		Status: "pending",
	}, nil
}

// GetRevocationStatus is the controller to get revocation status
func (s *Server) GetRevocationStatus(ctx context.Context, request GetRevocationStatusRequestObject) (GetRevocationStatusResponseObject, error) {
	return nil, nil
}

// PublishState is the controller to publish the state on-chain
func (s *Server) PublishState(ctx context.Context, request PublishStateRequestObject) (PublishStateResponseObject, error) {
	return nil, nil
}

// GetClaim is the controller to get a client.
func (s *Server) GetClaim(ctx context.Context, request GetClaimRequestObject) (GetClaimResponseObject, error) {
	if request.Identifier == "" {
		return GetClaim400JSONResponse{N400JSONResponse{"invalid did, can not be empty"}}, nil
	}

	did, err := core.ParseDID(request.Identifier)
	if err != nil {
		return GetClaim400JSONResponse{N400JSONResponse{"invalid did"}}, nil
	}

	if request.Id == "" {
		return GetClaim400JSONResponse{N400JSONResponse{"can not proceed with an empty claim id"}}, nil
	}

	clID, err := uuid.Parse(request.Id)
	if err != nil {
		return GetClaim400JSONResponse{N400JSONResponse{"invalid claim id"}}, nil
	}

	claim, err := s.claimService.GetByID(ctx, did, clID)
	if err != nil {
		switch err {
		case services.ErrClaimNotFound:
			return GetClaim404JSONResponse{N404JSONResponse{err.Error()}}, nil
		default:
			return GetClaim500JSONResponse{N500JSONResponse{err.Error()}}, nil
		}
	}

	return GetClaim200JSONResponse{
		Context: claim.Context,
		CredentialSchema: CredentialSchema{
			claim.CredentialSchema.ID,
			claim.CredentialSchema.Type,
		},
		CredentialStatus:  claim.CredentialStatus,
		CredentialSubject: claim.CredentialSubject,
		Expiration:        claim.Expiration,
		Id:                claim.ID,
		IssuanceDate:      claim.IssuanceDate,
		Issuer:            claim.Issuer,
		Proof:             claim.Proof,
		Type:              claim.Type,
	}, nil
}
