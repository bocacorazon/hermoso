# Historical documentation archive

This directory preserves design documents from before Hermoso's approved
TUI-first architecture. They are retained for provenance and context, not as
current requirements.

## Former DFT direction

Files under [`former-dft/`](former-dft/) describe earlier ideas in which the
project was named DFT and centered on Spec Kit workflow execution, autonomous
orchestration, evaluation, and self-improving skills:

- [`dft.md`](former-dft/dft.md)
- [`eval.md`](former-dft/eval.md)
- [`hermoso-flow-runner-plan.md`](former-dft/hermoso-flow-runner-plan.md)
- [`seed-of-a-seed.md`](former-dft/seed-of-a-seed.md)

These documents may contradict the current architecture. In particular, the
current product keeps Hermes as the primary UX, uses Hermes Kanban for dispatch,
and limits Hermoso to a deterministic Go control plane.

An earlier tracked `docs/spec-kit/` research set had already been removed before
this archive pass. It was not restored. The separate untracked
`research/spec-kit/` checkout is research material and was deliberately left
unchanged.
