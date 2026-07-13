Interview me relentlessly about every aspect of this plan until we reach a shared understanding. Walk down each branch of the design tree, resolving dependencies between decisions one-by-one. For each question, provide your recommended answer.

Ask the questions one at a time, waiting for feedback on each question before continuing. Asking multiple questions at once is bewildering.

If a fact can be found by exploring the codebase, look it up rather than asking me. The decisions, though, are mine — put each one to me and wait for my answer.

Do not enact the plan until I confirm we have reached a shared understanding.

## Completion

After shared understanding:

1. Write or update `docs/specs/<NNN>-<slug>.md` at the colony root (use the next available spec number under `docs/specs/`).
2. Emit `SIGNAL/spec.ready` with `ref` set to the repo-relative path of that file (required).
3. Optionally emit `INSIGHT/context.note` summarizing the Bloom for later bees.

Do not start breakdown, emit `task.plan`, or emit `task.ready` during grilling.
