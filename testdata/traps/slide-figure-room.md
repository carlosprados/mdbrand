---
title: "Room for a figure"
mdbrand:
  style: slides
---

<!-- A figure with paragraphs above it is the usual slide that does not fit.
     The build must stop, and say the width at which the figure leaves room. -->

## A diagram under its argument

The scheduler polls, the queue holds what it finds, and the sandbox runs each
job with its own quota.

Every arrow below is a place where a job can wait.

```d2 caption="Where a job goes"
direction: right
api: API
sched: Scheduler {
  poll: polling
  queue: queue
}
run: Sandbox
api -> sched.queue
sched.poll -> sched.queue
sched.queue -> run
```
