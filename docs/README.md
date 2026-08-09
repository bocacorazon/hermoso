# Hermoso documentation

Hermoso is the deterministic Go control plane for a development workflow whose
primary user experience is the Hermes TUI.

- [Architecture](architecture.md) — responsibilities, boundaries, phase
  contracts, Kanban dispatch, and the target `.hermoso` state.
- [Usage and workflow](usage.md) — current commands and the delivered
  design/construction workflow.
- [Implementation status](status.md) — what is available now and what remains
  implemented through construction; verification and release remain future.
- [Repository knowledge spine proposal](proposals/repository-knowledge-spine.md)
  — clean-room design for an evidence-backed, agent-navigable repository model.
- [Archive](archive/README.md) — historical documents from Hermoso's former
  DFT/Spec Kit direction.

The initial product scope is **design and construction**. Verification and
release exist in the domain lifecycle so persisted data can evolve without a
breaking redesign, but their operator workflows are not part of the current
implementation.
