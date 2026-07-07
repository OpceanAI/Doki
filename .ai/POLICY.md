# Doki AI Contribution Policy

This document governs any contribution to Doki produced with the help of AI
systems — coding agents, chat assistants, IDE agents, or scripted automation.
It applies equally to maintainers, external contributors, and automated
tooling operating on this repository.

This policy will be revisited as tooling, law, and community norms around AI
assistance continue to change. Its current form draws on practices already in
use across the Linux Kernel, Kubernetes, LLVM, and the OpenInfra Foundation.

## 1. Stance

Doki takes a **Boundary-and-Accountability** stance: AI assistance is
permitted and, in most cases, expected. What is not permitted is submitting
work you cannot explain, defend, or maintain yourself.

The standard is simple: a contribution must be worth more to the project than
the time it takes a maintainer to review it. Volume is not a substitute for
understanding, and a low review-to-generation cost ratio is not the goal.

AI may draft, refactor, translate, or explain. It does not approve, merge, or
release anything. That authority stays with human maintainers, without
exception.

## 2. Disclosure

Contributions that were meaningfully drafted, generated, or substantially
reshaped by an AI tool must disclose that fact. Trivial autocomplete or
single-line suggestions do not require disclosure; a function, a subsystem,
a bug fix, or a documentation section produced with AI assistance does.

Use the following trailer format in commit messages, following the pattern
adopted by the Linux Kernel, LLVM, and the OpenInfra Foundation:

```
Assisted-by: <tool name and version, if known>
Signed-off-by: <your name> <your email>
```

Use `Generated-by:` instead of `Assisted-by:` when the bulk of a specific
commit's content originated from an AI tool rather than being a human draft
refined with AI help.

**Do not use `Co-authored-by:` for AI tools.** Co-authorship implies shared
accountability that an AI system cannot hold. The `Signed-off-by:` line is
what matters here: it is your statement, as the human submitting the change,
that you take full responsibility for its contents, licensing, and
correctness — whether you wrote every line yourself or an AI tool helped
produce them.

Pull request descriptions for AI-assisted work should briefly note scope:
which parts were generated, which were hand-written, and what you changed
after generation. This is a courtesy to reviewers, not a formality — it
directly determines how much scrutiny a maintainer applies and how quickly
the review moves.

## 3. What AI must not do

- Approve, merge, tag, or publish a release.
- Invent APIs, platform support, CLI commands, or behavior not present in the
  repository. If it is not documented or implemented, it does not exist for
  the purposes of a contribution.
- Open pull requests against issues labeled `good-first-issue`. These exist
  to give new human contributors a path into the project; using automation
  against them defeats their purpose, following the precedent set by LLVM.
- Submit content the contributor has not personally reviewed and cannot
  explain line by line if asked.
- Weaken sandboxing, disable security checks, bypass validation, reduce
  isolation guarantees, alter the trust or identity model, expose new network
  surface, or execute shell commands built from untrusted input — without
  explicit maintainer instruction to do exactly that.

## 4. Source of truth

When sources disagree, resolve the conflict in this order:

1. Repository source code
2. Existing tests
3. CI results and locally reproducible validation
4. Repository documentation (README, wiki, CONTRIBUTING, architecture notes,
   release notes)
5. AI instruction files (`.ai/`)
6. This document

Undocumented, unimplemented behavior is not a valid basis for a contribution,
regardless of how confident an AI tool is about it.

## 5. Required workflow before modifying code

1. Read the documentation relevant to the affected subsystem.
2. Read `CONTRIBUTING.md` and this policy.
3. Keep the change scoped strictly to the task requested. Do not fold in
   unrelated refactoring, even if it looks like an improvement.
4. Update tests whenever behavior changes.
5. Update documentation whenever user-visible behavior changes.
6. Run the full verification suite below.
7. Produce a session report.
8. Submit for human review with disclosure per Section 2.

## 6. Required verification

Unless a maintainer explicitly says otherwise, every completed task must pass:

```bash
gofmt -w .
go build ./...
go vet ./...
go test -race ./... -v -count=1
golangci-lint run ./...
```

Changes touching security, concurrency, storage, networking, or runtime
selection require additional tests targeted specifically at the change, not
just the generic suite passing.

## 7. Branching and session reports

Each AI-assisted contribution uses its own branch:

```
ai/<agent>/<topic>
ai/<agent>/bugfix/<topic>
ai/<agent>/feature/<topic>
ai/<agent>/docs/<topic>
ai/<agent>/refactor/<topic>
```

Each branch includes a session report at `.ai/sessions/<branch>/AI_SESSION.md`
covering: agent name, model, date, branch, task summary, files modified,
prompt summary, key implementation decisions, tests executed, hardware and
platform tested, Doki version, commit hash, and review status.

## 8. Risk classification

**High risk** — requires explicit justification and careful human review:
`pkg/runtime/`, `pkg/network/`, `pkg/storage/`, `pkg/netlink/`,
`pkg/emulation/`, `pkg/api/`, `internal/proot/`, `internal/seccomp/`,
`internal/apparmor/`, `internal/namespaces/`, `internal/cgroups/`; any code
executing external binaries, handling untrusted paths, parsing image
metadata, or constructing command arguments. These changes must preserve
compatibility unless a breaking change was explicitly requested.

**Medium risk**: `cmd/`, `pkg/compose/`, `pkg/image/`, `pkg/registry/`,
`pkg/cli/`, `internal/` in general.

**Low risk**: `docs/`, `.wiki/`, `README.md`, `CONTRIBUTING.md`, `scripts/`,
`test/`, `*_test.go` files.

The closer a change gets to isolation decisions, external binary execution,
or cryptographic trust and identity, the higher the bar for review. The
closer it gets to documentation or test scaffolding, the more room there is
to move quickly.

## 9. Coding standards

Structured logging via `log/slog`. Explicit error returns rather than
panic-driven control flow — constructors return `(*T, error)`. Persistent
configuration files are written atomically. New dependencies require clear
justification; do not add one to save a few lines of code. Exported symbols
and user-facing commands are not renamed unless requested. Synchronization or
locking is not simplified without understanding the full execution path.
Platform-specific functionality goes behind build tags.

## 10. Testing

Every behavioral change needs a corresponding test. Cover the success path
and at least one failure path; when a function has multiple decision
branches, each branch gets its own test rather than one broad test covering
several at once. Tests must be deterministic and pass with the race detector
enabled (`go test -race`). Use descriptive, specific names —
`TestRegistryBestForChoosesHighestUsableLevel`, not `TestRegistry1`.
Platform-specific changes document which platform, which architecture, and
what limitations remain known.

## 11. Documentation

User-visible changes update the corresponding documentation — README, wiki,
release notes, examples, or templates as applicable. Documentation describes
what is implemented, not what is planned. Do not present experimental
functionality as production-ready.

## 12. Security

Security-sensitive code is modified conservatively. AI contributors must not
weaken sandboxing, disable security checks, bypass validation, reduce
isolation guarantees, change the trust or identity model, expose new network
surface, or introduce shell execution using untrusted input, without explicit
instruction to do exactly that. Session reports touching security document
the risk, the mitigation applied, and what validation was performed.

## 13. Stop and ask

Stop and request clarification whenever: repository documentation conflicts
with itself, a requirement is ambiguous, a breaking API change appears
necessary, unsupported platform behavior would be required, there is
insufficient evidence to determine the correct implementation, or a
high-risk subsystem needs modification without explicit approval. Do not
proceed on a guess in any of these cases.

## 14. Expected deliverable

Before proposing a change as complete, summarize: what changed, which files,
which tests were run, what uncertainties remain, and the review status.
Every change should be easy for a human to understand, audit, and revert.

## 15. Human authority

Human maintainers hold final authority over architectural decisions, security
decisions, releases, merges, roadmap changes, and compatibility guarantees.
AI is an implementation assistant. It is not a maintainer, does not accrue
authorship in the project's governance sense, and does not carry any of the
accountability that a human contributor takes on by signing off a commit.

