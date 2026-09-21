# Approval Gate prototype (historical)

`shell.qml` in this directory is the throwaway, hardcoded-sample-data
prototype built to settle the Approval Gate's chrome (wayfinder ticket #15 /
[decision record](https://github.com/BasantPandey/CmdWarden/issues/15)):
three layout variants (Compact Toast, Center Modal, Queue Panel), switchable
at runtime, with **no real Session Agent wiring**.

It was superseded by the real, D-Bus-wired implementation in
[`internal/agentd/qml/shell.qml`](../../../internal/agentd/qml/shell.qml)
(ticket #6), which took Variant 2 (Center Modal) and replaced every
hardcoded field with live data from the agent. Kept here only as the design
record for *why* Variant 2 looks the way it does and what the alternatives
were — not meant to be run against anything real.
