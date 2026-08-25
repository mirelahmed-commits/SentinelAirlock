# Early User Feedback Guide

## What to ask

1. Install friction: where did setup slow down?
2. Agent setup friction: was backend readiness obvious?
3. Replay trust clarity: did timeline explain what happened?
4. Policy clarity: did users understand allow/deny outcomes?
5. Sandbox surprises: did runtime/network behavior match expectation?
6. Remote workflow pain: submit/fetch/worker clarity gaps?
7. Reviewer UX clarity: can users decide "approve/reject" quickly?

## What to measure

- Time from install to first successful run
- Time from run completion to review decision
- Frequency of verification confusion
- Frequency of policy misconfiguration confusion
- Remote flow success rate

## Highest-priority bug classes

- Wrong policy decision/revert behavior
- Inconsistent verify state across CLI/web
- Broken evidence artifacts (digest/signature/export)
- Worker auth or artifact fetch failures
- Replay mismatch with manifest

## What not to promise yet

- Full enterprise IAM
- Fully universal sandbox guarantees on every host/runtime
- Hosted control-plane capabilities
