---
title: "Decks & frames"
subtitle: "Every shape a slide has got wrong"
author: "mdbrand"
date: "October 2026"
lang: en-GB
mdbrand:
  brand: deck
  style: slides
  confidential: "Internal"
---

# Figures

## A diagram — wide and low

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

## A chart

```vegalite caption="Jobs per second"
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 460, "height": 150,
  "data": {"values": [
    {"n": "1", "jobs": 4200}, {"n": "2", "jobs": 8100},
    {"n": "4", "jobs": 15600}, {"n": "8", "jobs": 29800}]},
  "mark": "bar",
  "encoding": {
    "x": {"field": "n", "type": "nominal", "title": "nodes"},
    "y": {"field": "jobs", "type": "quantitative", "title": "jobs/s"}},
  "config": {"axis": {"labelFontSize": 15, "titleFontSize": 15, "labelAngle": 0}}
}
```

# Text

## A table

A table loads the caption package, which numbered every caption on the deck
until mdbrand set the label format itself.

| Pattern | p99 | Tenants |
|---|---|---|
| Cron | 60 s | no |
| Queue and workers | 200 ms | partly |
| Sniper polling | 15 ms | yes |

## Code beside a list

:::: columns
::: column
```go
func run(ctx Ctx) error {
	r := ctx.Get(ctx.URL)
	return r.Err
}
```
:::
::: column
- No `require`, no open network
- CPU quota per tenant
  - and a nested point
:::
::::

::: notes
Speaker notes: never on the slide.
:::

## A long path

The repository `wolf-code/0-active/wolfops/internal/scheduler/sniper-polling-window.go`
breaks after its separators, as it does on the page, a
[link](https://github.com/carlosprados/mdbrand) is coloured, and an
[accented phrase]{.accent} prints in the bundle's colors.accent.
