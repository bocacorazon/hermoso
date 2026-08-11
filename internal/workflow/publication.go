package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/bocacorazon/hermoso/internal/digest"
	"github.com/bocacorazon/hermoso/internal/domain"
	"github.com/bocacorazon/hermoso/internal/model"
)

type publicationOriginal struct {
	path    string
	data    []byte
	mode    os.FileMode
	existed bool
}

func (s Service) publishPassingGherkin(
	ctx context.Context,
	execution domain.ContextRef,
	run domain.Run,
	attempt domain.VerificationAttempt,
) (result domain.Run, resultErr error) {
	construction := run.LatestConstruction()
	if construction == nil {
		return domain.Run{}, errors.New("passing verification has no construction round")
	}
	feature := repoWorkspace(construction.Feature)
	currentCommit, err := s.Manager.CurrentCommit(feature)
	if err != nil {
		return domain.Run{}, err
	}
	assets, err := s.Store.ReadSealedArtifacts(ctx, execution, run.Design.ArtifactRootHash)
	if err != nil {
		return domain.Run{}, err
	}
	var originals []publicationOriginal
	var publishedPaths []string
	approvedContent := map[string][]byte{}
	committed := false
	defer func() {
		if resultErr == nil || committed {
			return
		}
		for _, original := range originals {
			if original.existed {
				_ = os.WriteFile(original.path, original.data, original.mode)
			} else {
				_ = os.Remove(original.path)
			}
		}
	}()
	for _, artifact := range run.Design.Verification.Artifacts {
		if artifact.Kind != "gherkin" {
			continue
		}
		content, ok := assets[artifact.Path]
		if !ok || digest.Bytes(content) != artifact.ContentHash {
			return domain.Run{}, fmt.Errorf("approved Gherkin artifact %q is missing or changed", artifact.ID)
		}
		destination := filepath.Join(feature.Path, filepath.FromSlash(artifact.PublicationPath))
		relative, err := filepath.Rel(feature.Path, destination)
		if err != nil || filepath.ToSlash(relative) != artifact.PublicationPath {
			return domain.Run{}, fmt.Errorf("publication path %q escapes the feature worktree", artifact.PublicationPath)
		}
		approvedContent[artifact.PublicationPath] = content
		publishedPaths = append(publishedPaths, artifact.PublicationPath)
	}
	sort.Strings(publishedPaths)
	if len(publishedPaths) == 0 {
		return domain.Run{}, errors.New("passing verification has no approved Gherkin to publish")
	}
	publicationCommit := currentCommit
	if currentCommit == attempt.CandidateCommit {
		if err := s.Manager.WorktreeClean(feature); err != nil {
			return domain.Run{}, fmt.Errorf("publish Gherkin into clean feature worktree: %w", err)
		}
		for _, path := range publishedPaths {
			destination := filepath.Join(feature.Path, filepath.FromSlash(path))
			original := publicationOriginal{path: destination, mode: 0o644}
			if info, err := os.Stat(destination); err == nil {
				original.existed, original.mode = true, info.Mode().Perm()
				original.data, err = os.ReadFile(destination)
				if err != nil {
					return domain.Run{}, err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return domain.Run{}, err
			}
			originals = append(originals, original)
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return domain.Run{}, err
			}
			if err := os.WriteFile(destination, approvedContent[path], 0o644); err != nil {
				return domain.Run{}, err
			}
		}
		publicationCommit, err = s.Manager.CommitAllowedFiles(
			ctx, feature, publishedPaths,
			fmt.Sprintf(
				"Publish verified behavior for %s\n\nHermoso-Run: %s\nHermoso-Attempt: %d\nHermoso-Package: %s",
				execution.FeatureID, execution.RunID, attempt.Number, attempt.PackageHash,
			),
		)
		if err != nil {
			return domain.Run{}, err
		}
	} else {
		publicationCommit, err = s.Manager.ValidateAllowedCommit(
			feature, attempt.CandidateCommit, publishedPaths,
		)
		if err != nil {
			return domain.Run{}, fmt.Errorf("resume Gherkin publication: %w", err)
		}
		for _, path := range publishedPaths {
			content, err := os.ReadFile(filepath.Join(feature.Path, filepath.FromSlash(path)))
			if err != nil {
				return domain.Run{}, err
			}
			if digest.Bytes(content) != digest.Bytes(approvedContent[path]) {
				return domain.Run{}, fmt.Errorf("published Gherkin path %q does not match the approved artifact", path)
			}
		}
	}
	committed = true
	if publicationCommit == attempt.CandidateCommit {
		return domain.Run{}, errors.New("Gherkin publication did not create a distinct commit")
	}
	project, err := s.Store.Project(ctx)
	if err != nil {
		return domain.Run{}, err
	}
	snapshot, err := model.Build(ctx, model.BuildRequest{
		Project: project, RepositoryRoot: project.Target.Repository,
		Revision: publicationCommit, GeneratedAt: s.Now().UTC(),
	})
	if err != nil {
		return domain.Run{}, fmt.Errorf("refresh repository model after Gherkin publication: %w", err)
	}
	snapshot, err = model.AddPublishedScenarios(
		snapshot, run.Design.Feature, *run.Design.Verification,
		attempt.Report.SurfaceResolutions,
	)
	if err != nil {
		return domain.Run{}, err
	}
	snapshot, _, err = s.Store.PutModelSnapshot(ctx, snapshot)
	if err != nil {
		return domain.Run{}, err
	}
	publication := domain.GherkinPublication{
		Attempt: attempt.Number, VerifiedCommit: attempt.CandidateCommit,
		PublicationCommit: publicationCommit, Model: modelReference(snapshot),
		PublishedPaths: publishedPaths, PublishedAt: s.Now().UTC(),
	}
	return s.Store.UpdateRun(ctx, execution, func(current *domain.Run) error {
		if current.Publication != nil {
			if current.Publication.PublicationCommit == publicationCommit {
				return nil
			}
			return errors.New("a different Gherkin publication is already recorded")
		}
		if len(current.VerificationAttempts) != int(attempt.Number) ||
			current.VerificationAttempts[attempt.Number-1].ReportHash != attempt.ReportHash {
			return errors.New("passing verification attempt changed during publication")
		}
		current.Publication = &publication
		current.Phase = domain.PhaseVerification
		current.Status = domain.StatusAwaitingRelease
		current.VerificationBlocker = ""
		current.Revision++
		current.UpdatedAt = s.Now().UTC()
		return nil
	})
}
