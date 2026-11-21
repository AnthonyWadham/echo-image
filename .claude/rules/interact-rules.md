---
paths:
  - "shards/game/internal/interact/**"
  - "shards/game/internal/projectile/**"
  - "shards/game/internal/area/**"
  - "shards/game/internal/effect/**"
  - "shards/game/content/projectile/**"
  - "shards/game/content/area/**"
  - "shards/game/content/effect/**"
---

# Rules: what happens when X meets Y (`interact`)
- **Every "X meets Y" is data: a filter plus a response, and the first rule that matches wins.** The
  vocabulary is `interact`: `Kind` (what a thing is — the Box2D category bits), `Tags` (states effects
  grant, e.g. `stunned`), `Relation` (`own`, `ally`, `enemy`, `neutral`, from each side's `team.Owner`
  and `team.Team`), and `Filter{Kinds, Relations, Require, Forbid}` (an empty field matches anything).
  Content names them in JSON lists; an unknown name fails the boot.
- **The GDD's rules are default content, not code.** A shot type's own `meets` rules are checked first,
  then `defaults` in `content/projectile/tuning.json` (walls and bounds stop, own and ally pass, `out`
  Bodies pass, Bodies stop, enemy Echoes take the payload). So "some shots pass Bodies" is one rule on that shot type. Order
  is the only priority; the rule index breaks ties between things met at the same point. A rule with a
  payload must name its relations, since a shot type's own rules come before the defaults' own and ally
  pass: friendly fire is written out, never a default.
- **Executors switch on responses and options, never on content IDs.** `projectile.FlightSystem` (shot
  `meets`: `stop`, `pass`, payload yes or no) and `area.AreaSystem` (area rules) are the executors
  today. A new response is an enum value with a JSON name plus one branch in its executor, tested
  against the others.
- **Tags are a closed Go list, append only** (snapshots keep the bits). Add one only when several
  features read it; state one champion alone reads stays in its own component. What a tag does to
  other effects is data: an effect kind's `ends` (the holder's entries it removes as it lands) and
  `blocked_by` (holder tags that stop it), and each ability's `blocked_by`, or the file's (the cast gate).
- **Areas test who is inside every tick.** An `area/component.Area{Kind, Start}` makes its entity's
  first shape act on what overlaps it: one Box2D circle overlap (`Physics.Engine().OverlapShape`), then
  the first matching rule per thing. Bodies and Echoes mask `interact.Area` so the query finds them. Anti-camp is
  the Echo's area (`content/area/tuning.json`, kind `anticamp`), and its `Start` is the warmup.
