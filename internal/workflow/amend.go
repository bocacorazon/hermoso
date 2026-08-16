package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/bocacorazon/hermoso/internal/contracts"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/verification"
)

// isAmendable returns true when a run is in a state where the verification
// contract can be amended (corrected test artifacts without redoing construction).
func isAmendable(run domain.Run) bool {
	if run.Design == nil || run.Design.Verification == nil {
		return false
	}
	switch {
	case run.Phase == domain.PhaseVerification && run.Status == domain.StatusBlocked:
		return true
	case run.Phase == domain.PhaseConstruction && run.Status == domain.StatusAwaitingVerification:
		return true
	default:
		return false
	}
}

// AmendVerification accepts a corrected verification contract, re-seals the
// corrected artifacts, clears old verification history, and transitions to
// awaiting_verification. No re-approval or re-construction is needed — the
// feature design is unchanged, only the test artifacts are corrected.
//
// Amendment is for test defects (wrong API assumptions in sealed tests),
// distinct from remediation (code defects). The new contract gets a fresh
// 2-attempt budget because the old attempts tested a different contract.
func (s Service) AmendVerification(
	ctx context.Context,
	execution domain.ContextRef,
	data []byte,
	assets map[string][]byte,
) (domain.Run, bool, error) {
	var contract domain.FeatureVerificationContract
	if err := decodeContract(contracts.FeatureVerificationContract, data, execution, &contract); err != nil {
		return domain.Run{}, false, err
	}
	current, err := s.Store.Run(ctx, execution)
	if err != nil {
		return domain.Run{}, false, err
	}
	if !isAmendable(current) {
		return domain.Run{}, false, errors.New("verification amendment requires a run in blocked verification or awaiting_verification state")
	}
	if current.Design == nil || current.Design.Verification == nil {
		return domain.Run{}, false, errors.New("verification amendment requires an existing verification contract")
	}
	if contract.Revision <= current.Design.Verification.Revision {
		return domain.Run{}, false, fmt.Errorf("amendment revision %d must exceed current revision %d", contract.Revision, current.Design.Verification.Revision)
	}
	snapshot, err := s.Store.ModelSnapshot(ctx, current.Design.Feature.BaseModel.SnapshotID)
	if err != nil {
		return domain.Run{}, false, fmt.Errorf("load model for amendment: %w", err)
	}
	if err := verification.ValidateContract(
		contract, current.Design.Feature, current.Design.FeatureHash, snapshot, assets,
	); err != nil {
		return domain.Run{}, false, err
	}
	artifactRootHash, err := s.Store.SealArtifacts(ctx, execution, assets)
	if err != nil {
		return domain.Run{}, false, err
	}
	contractHash, err := Hash(contract)
	if err != nil {
		return domain.Run{}, false, err
	}
	changed := false
	run, err := s.Store.UpdateRun(ctx, execution, func(run *domain.Run) error {
		if !isAmendable(*run) {
			return errors.New("run is no longer amendable")
		}
		// Clear all verification history — old attempts and incidents tested
		// a different (buggy) contract. Their results are irrelevant to the
		// corrected contract. The new contract gets a fresh 2-attempt budget.
		run.VerificationAttempts = nil
		run.VerificationIncidents = nil
		run.VerificationBlocker = ""
		// Recompute the design package hash (it includes the verification hash)
		// and update all bound entities so validation passes.
		packageRevision := run.Design.Revision + 1
		packageHash, err := domain.DesignPackageHash(
			packageRevision, run.Design.FeatureHash, contractHash,
			artifactRootHash, run.Design.Feature.BaseModel,
		)
		if err != nil {
			return err
		}
		run.Design.Revision = packageRevision
		run.Design.PackageHash = packageHash
		run.Design.Verification = &contract
		run.Design.VerificationHash = contractHash
		run.Design.ArtifactRootHash = artifactRootHash
		// Update approval to match the new package
		if run.Design.Approval != nil {
			run.Design.Approval.Revision = packageRevision
			run.Design.Approval.PackageHash = packageHash
		}
		// Update construction round 0 source_hash to match the new package
		if len(run.ConstructionRounds) > 0 {
			run.ConstructionRounds[0].SourceHash = packageHash
			// If there was a remediation round, fold its integrated commit
			// into round 0 so the latest code is what gets verified.
			if len(run.ConstructionRounds) > 1 {
				last := run.ConstructionRounds[len(run.ConstructionRounds)-1]
				if last.IntegratedFeatureCommit != "" {
					run.ConstructionRounds[0].IntegratedFeatureCommit = last.IntegratedFeatureCommit
				}
			}
		}
		// Drop remediation rounds — they reference the old verification contract
		// and its failed attempts, which no longer exist after the reset.
		if len(run.ConstructionRounds) > 1 {
			run.ConstructionRounds = run.ConstructionRounds[:1]
		}
		// Clear task bindings for dropped remediation rounds (keep round-0 only)
		round0Bindings := make([]domain.TaskBinding, 0, len(run.TaskBindings))
		for _, tb := range run.TaskBindings {
			if tb.Round == 0 {
				round0Bindings = append(round0Bindings, tb)
			}
		}
		run.TaskBindings = round0Bindings
		// Transition to awaiting_verification (construction is still valid)
		run.Phase = domain.PhaseConstruction
		run.Status = domain.StatusAwaitingVerification
		run.Revision++
		run.UpdatedAt = s.Now().UTC()
		changed = true
		return nil
	})
	return run, changed, err
}
