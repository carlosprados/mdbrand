Documents that must NOT build cleanly.

Each one provokes exactly one failure or warning, and `scripts/torture.sh`
asserts on the words it comes out with. The message is the feature here: a build
that stops without naming the fix is the defect this directory exists to catch.
