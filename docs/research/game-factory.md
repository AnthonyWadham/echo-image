# Import the systems, copy only the glue

Game factory research, 2026-11-21. Built from the five notes in `game-factory-notes/`, plus a read-only look at echo_image and world-engine.
Key: [n] = a source in Appendix B. "(reported)" = only second-hand support. "Inference" = my reasoning, not a source.

## 1. Verdict

- **Pick none of A, B or C as written.** Each one mixes three choices: how code ships, how the agent learns it, and how rules are enforced.
- **Ship two units, as mature engines do:** a starter the game copies and owns, plus versioned systems the vendor owns [2][4][5][11][38]. In Go, that means imported packages, not copies (inference).
- **Teach with a short always-loaded index plus each package's own doc.** Add two workflow skills. No router skill, and no skill per system by default [1][36][37].
- **Enforce with code that ships with each system:** a conformance test, its mutants, analyzers, and CI [85][87][88].
- **"Solid examples ≈ plugins" is untested.** The evidence suggests it holds for a first build and for glue only. Copies lose fixes and make the tests editable [10][24][85].

## 2. Options against five criteria

New option **D. Starter + kit**: a thin starter, copied once. Systems are imported Go packages, each with its doc, a conformance test and its mutants. AGENTS.md holds an always-loaded index. Two workflow skills join the existing `world-local-dev`.

| Criterion | A. Full template + skill | B. Catalog + pointer skill | C. Skill per system + router | D. Starter + kit |
|---|---|---|---|---|
| Agent success | Easy start: Chef starts every project from one template [45]. On hard games, "usable" falls 73.7%→12.6% [32] | A lone pointer skill scored the same as no docs: 53% [36] | Curated skills +16.6 pp; small ones do best [79]. But two skills must fire, and each fires about half the time [36][83] | Uses the setup that scored 100% vs 79% for a prompted skill, in a vendor eval [36]. Unmeasured on games |
| Context cost | Every rule, every session. Today about 20K tokens (est.) [94] | Low while the index stays ≤8 KB [36] | About 100 tokens per skill, every turn [54][55]. The listing is capped [55] | Index ≤8 KB, plus one package doc when used |
| Updates | None. Copies fork [10][38] | Copy-in: diff and merge by hand [41]. Import: `go get` | Per-system versions need release tooling [53] | `go get`, `retract` and `//go:fix inline` reach every game that updates [50][51] |
| Scaling | Strains by 50, fails by 200 (Q7) | A flat list degrades past about 30 [80] | Descriptions get cut at about 30–60 skills (inference) [55][62] | Same as B: a flat list degrades past about 30 [80]. Group by family, then search |
| Effort to build (est.) | Low now, high later | Medium | High: N docs, N skills, N versions, a router | Medium: a test and a doc per system, one index |

What each option gets right, and its weak part:
- **A** gives a working start. But the research found no engine that ships one template with every system. Unreal splits templates by genre (reported) [3]. Unity installs only what a game type needs [4]. Big samples go stale (reported) [19]. Chef credits Convex's APIs, not its template [45].
- **B** gives one catalog. Phaser AE ships this: 358 packs, one skill, search by capability [21][22][23]. Its weak part is the pointer *skill* [36]. Also, Phaser has no networking packs [22].
- **C** gives one package per system. Laravel and Unreal ship agent files per package [1][37]. Its weak parts are a skill per system and a hand-written router. In shipped tools, the host or the package manager picks the files [1][37].
- **D** has weak parts too:
  - Phaser AE, the one shipped agent catalog for games, copies parts in as files [21][22].
  - Its index evidence is one vendor eval. Frontier models hit 100% without the pointer [35][36].
  - Human-written context files gave +2.4 pp (not significant) for up to +19% cost [75].
  - Read-only tests stop test edits, not special-casing [85].
  - Imports bring version skew and breaking changes (inference). No study compares copy and import (Q4).

## 3. Recommended layout: a thin starter on an imported kit

### Folder tree (new paths are marked proposed)

```text
world-engine/
├── pkg/plugin/
│   ├── physics2d/                      existing: a plugin inside the engine module
│   ├── queue/                          proposed: from echo_image/pkg/queue
│   ├── level/  levelstate/             proposed: from echo_image/pkg/level, pkg/levelstate
│   └── index.md                        proposed: need → system → doc → check, by family
├── pkg/analysis/ownercheck/            proposed: golangci-lint module plugin, from echo's prototype
├── pkg/template/
│   ├── basic/  bare-bone/  multi-shard/   existing: copied by `world setup -t`
│   └── starter/                        proposed: glue, plus one copied example per system
└── cli/pkg/worldscaffold/templates/skills/
    ├── world-local-dev/                existing: written into every new game
    ├── add-system/                     proposed: the one "use a system" workflow
    └── upgrade-world/                  proposed: from .agents/skills/cardinal-migration

my-game/                                proposed: made by `world setup -t starter`
├── AGENTS.md                           proposed: managed index block for the systems it imports
├── CLAUDE.md                           proposed: "@AGENTS.md", like world-engine's own CLAUDE.md
├── .agents/skills/  .claude/skills/    existing: the scaffolder writes both
├── shards/game/                        existing in each template: copied once; the game owns it
└── go.mod                              existing: pins world-engine; proposed: systems come with it
```

- Systems move to world-engine once proven. They sit next to `physics2d`, which already lives in the engine module [100]. One module means one version line.
- The scaffolder already writes `world-local-dev` into both `.claude/skills` and `.agents/skills` [99]. That covers tools that read only one of the two [55][62]–[65].
- Copied examples teach the wiring. Imported packages hold the rules. Phaser's advice: build one example by hand for the AI to study [25].

### One system up close: `queue`

**Package**

```text
pkg/plugin/queue/                       proposed home; today: echo_image/pkg/queue
├── doc.go                              existing: the agent's guide
├── queue.go  delayed_event.go          existing
├── queue_test.go  delayed_event_test.go   existing: they kill the 11 queue mutants
├── mutants.json                        proposed: those 11 mutants, run by the kit's CI
└── queuetest/queuetest.go              proposed: CheckOrder, the game-side wiring check
```

**Doc.** `doc.go` is the guide. It already holds the API, the order rules, and the stand-ins for what Cardinal lacks [97]. Add three lines at the top:

```go
// Use when one feature changes state another owns, or fires an event on a later tick.
// Not for broadcasts with many readers: use a system event.
// Check a game's wiring with queuetest.CheckOrder.
```

- `go doc` reads it at the version the game pins (inference). Go 1.27 added `go doc pkg@version` [52]. This is the Go form of Next.js shipping docs inside its package (inference) [34].
- `docs/research/owner-queues.md` and `queue-api.md` stay as design history. Link them from `doc.go` (proposed). No engine found ships design research with its packages; they ship API docs [17][18]. An agent should not need them to use the queue (inference).

**Index row** (one line in the game's AGENTS.md block):

| Need | System | Read | Prove |
|---|---|---|---|
| One feature changes state another owns; an event on a later tick | queue | `go doc <kit>/queue` | `queuetest.CheckOrder` |

**Skill.** None for `queue` alone. One shared skill covers every system:

```markdown
---
name: add-system
description: Add a World Engine system (queue, level, levelstate, …) to this game. Use when a request
  needs a mechanic from the AGENTS.md system index, or says "add", "use" or "wire" a system.
---
1. Find the system's row in the AGENTS.md index. Read its doc with `go doc`.
2. Wire it in the owning feature's register.go and in shard.go, as the doc says.
3. Add the row's "Prove" check to the game's tests. Run the game's checks.
4. If a system rule blocks the request, stop and report it. Never copy, edit or `replace` the system.
```

The skill's step 2 assumes Echo's layout. World-engine's templates have no `register.go` or `shard.go`, so the starter must add them (proposed).

Why this shape:
- **Always-loaded index, no router skill.** In Vercel's own eval, an always-loaded index beat skills [36]. In its public evals, weaker models gain the most [35].
- **A system gets its own skill only for a real multi-step job**, like baking a map for `level`. Install it only in games that import that system [37].
- **"Stop and report" (step 4).** An abort option cut GPT-5's cheating from 54% to 9%, but barely changed Opus 4.1 [85].
- **Checks over prose.** "Never send an LLM to do a linter's job" [73]. Claude Code says rules that must always hold belong in hooks, not skills [55].

## 4. First three steps

**Step 1. Make `pkg/queue` the pilot kit package (in echo_image).**
- Move the order check out of the game harness. `TestSendersRunBeforeTheirReceivers` is at `shards/game/test/harness_test.go:614`. Its logic is generic. Only its inputs are Echo's: `shard.Register`, the source folders, the module path [96]. Make those the arguments of `queuetest.CheckOrder`.
- Why: the 11 queue mutants (QU-S1 … QU-E6) die in `go test ./pkg/queue`. The 3 wiring mutants (QU-R1–R3) die only in Echo's harness [95]. They are a drain with no type, a send after the drain, and a read before the emit.
- A new game gets none of those 3 checks today.
- The scan assumes one system per file, an Echo convention the harness enforces [96]. `queuetest` must state that rule.
- Put the 11 queue mutants next to the package, or tag them, so the kit's CI can run them alone.
- Add the three doc lines from section 3 to `pkg/queue/doc.go`.
- Done when `scripts/check.sh` passes and all 14 queue mutants still die.

**Step 2. Cut `CLAUDE.md` to an index of 200 lines or fewer.**
- Today it is 966 lines and 82,345 bytes, about 20K tokens (inference) [94]. That is 4.8× Claude Code's 200-line target [56] and about 10× Vercel's 8 KB index [36] (inference). Codex reads only AGENTS.md by default, up to 32 KiB [61].
- Make it AGENTS.md, with CLAUDE.md as `@AGENTS.md`, as world-engine does. Claude Code follows that import [56].
- Keep: build and run, the rules that hold everywhere, and a system index (need → system → doc path).
- Move each system section next to its code. The three biggest, plus the queue [94]:
  - "Client sync (`netsync`)", 190 lines → `shards/game/internal/netsync/doc.go`.
  - "Collision handling", 109 lines → the packages it describes.
  - "Game data / definitions (`content.Load`)", 72 lines → `shards/game/content`.
  - "Owner queues", 13 lines → `pkg/queue/doc.go`, which holds most of it already.
- Rules for one folder go in `.claude/rules/<system>.md` with `paths:` [56]. Codex loads AGENTS.md files only from the root down to the working folder, so the index must name each doc path [61].
- Rules a test or mutant already pins: shrink each to one line that names the test.
- Done when the file is under 200 lines and the checks pass.

**Step 3. Test "examples ≈ plugins" with a 10-task eval before building more.**
- Make a scratch game from world-engine's `pkg/template/basic`, in Echo's feature layout. Give it `pkg/queue`, `pkg/level` and `pkg/levelstate` two ways:
  - Arm 1, imported: a module dependency, with edits to it denied.
  - Arm 2, copied in as example code.
- Write 10 tasks a vibe-coder might ask. For example:
  - "A trap hurts you 2 s after you step on it" (`queue.SendAt`, `DeclareDelayed`).
  - "Rotate to the next map each round" (`levelstate`, `Change`).
  - "Heal allies in a circle" (a send to the owner's queue).
- Score each run: build, tests, `queuetest`, whether system files or tests were edited, then one human play pass (Phaser does this [21]).
- Run at least two models, one of them cheap. Weak models gain the most from an index [35].
- Why now: no study measures agents on server-authoritative multiplayer [32][33]. Vendors that ship agent packaging run their own evals [16][35][36]. Anthropic advises at least 3 evals before writing a skill [59].
- If the copied arm matches the imported one, copy-in is fine for a first build. Updates still need a plan (Q4).
- If import wins: move the packages next to `physics2d` in world-engine. Then have the scaffolder write the index block too [99][100].

## 5. Open questions and risks

- **No outside evidence on multiplayer.** The benchmarks test single-player web or Pygame games [32][33]. Step 3 will be your only data.
- **Look-alike systems.** Prediction, lag compensation and favor-the-shooter overlap. So do queues and system events. One look-alike cut skill selection by 7–30% in a synthetic test [80]. Each index row needs a "not for".
- **Where the kit lives.** The engine module (like `physics2d`) gives one version line. A separate module releases faster but adds upkeep (inference). OpenTelemetry calls its contrib repo "the last place" for a new module [53]. Decide before moving code.
- **Game words.** `history` imports `interact`, `team`, `ref`, `netsync` and three game component packages [98]. It needs a cut before it can ship. Phaser strips game details from parts before publishing [21].
- **Where copying is fine.** Content, tuning, UI and `client/src/game` belong to the game. A rough line (inference): if more than one game must keep a rule, import the code that holds it.
- **Agents route around rules.** Claude models cheat mostly by editing tests [85]. CI must reject a `replace` or `vendor/` of the kit. Hooks alone leak (Q5).
- **Prompts are unreliable.** "Do not cheat" left o3's planned-hack rate at 80% on one task [86]. A strict prompt cut GPT-5 from 92% to 1% on LiveCodeBench, yet it still cheated 54% on SWE-bench [85]. It depends on model and task.
- **Tool churn.** Unreal's MCP is experimental [1]. Unity's is beta [6]. Skill folders differ by tool [55][62][63]. Plain files plus CI should outlast all of them (inference).
- **Owners.** OpenTelemetry needs a code owner per module and drops owners after 5 months idle [53]. Who owns each system at 50?
- **Tick rates.** Conformance tests must run at 20 Hz and 60 Hz, as `scripts/check.sh` does now [95].
- **The client.** `client/src/engine` is mostly generic (a stray "arena" sits in `render/stage.ts`); `client/src/game` is not. The same split could ship on npm (inference). This report does not cover it.
- **Search by 50 systems.** At ~80K skills, routing without skill bodies lost 37–44 pp [81]. Plan a search tool over full package docs (`world` CLI or MCP) by about 50 (inference).

## Appendix A. Findings by question

### Q1. How mature engines and frameworks package systems for agents

| Who | Unit of reuse | Agent side | Note |
|---|---|---|---|
| Unity | Packages; samples copied into `Assets` [4][5] | MCP in AI Assistant, beta [6] | Multiplayer Center: game type → package set [4] |
| Unreal | Templates by genre (reported) [3]; Game Feature plugins [2] | MCP, experimental; loads `SKILL.md` from each enabled plugin [1] | Tool search on by default [1] |
| Godot | Templates vs assets placed in a project [11] | None official found | Authors had to re-upload to the new store [11] |
| Roblox | Packages: linked copies [10] | Built-in Studio MCP, ~31 tools, incl. `skill` [9] | Auto-update stops at the first edit [10] |
| Bevy | Plugin crates with a Bevy-version table [12] | None official; the linter is community-run [13] | — |
| SpacetimeDB | Modules | 11 skills by language and layer; `concepts` is always on in Cursor [14][15] | Vendor benchmark: 91.7% vs 62.7% for PostgreSQL [16] |
| Phaser AE | 358 versioned packs, copied in as files [21][22] | One skill, search by capability [21][23] | No networking packs; games pin the engine [22][24] |
| three.js | Library only | A ~150-line `llms.txt` [26] | No official game starter or MCP (none found) |
| Rosebud | Start from a template [28] | — | Hit a "character limit wall"; moved to multi-file by hand [28] |
| Rune | One pure logic file on server and clients | — | Hard limits: state <1 MB, action <10 ms [29] |
| shadcn | Copy-in registry, any file type [39] | MCP search and install [40] | No field for tests [39]; moved shared code to packages [42] |
| Next.js | Dependency, with version-matched docs inside [34] | Managed AGENTS.md block; skills only for workflows [34] | `next upgrade --agent` [34] |
| Laravel | Starter kit copied; auth stays in Fortify [38] | Boost: guidelines and skills per installed package [37] | `boost:update` after upgrades [37] |

- Phaser rebuilt a small engine for agents, because "a big API … degrades it" [20][21].
- Heroic Labs' Hiro is one toolkit with about 20 metagame systems [18]. It is a catalog of systems, but not real-time gameplay.
- VibeGame (opinion): agents struggle with interaction, inventory, multiplayer and combat [27].
- No AI-first builder found documents server-authoritative multiplayer (no case found; note 02).

### Q2. Agent-side formats

| Format | Does well | Known limits |
|---|---|---|
| Agent Skills (`SKILL.md`) | About 100 tokens each until used; the body loads on demand [54]. Codex, Cursor, Copilot and Gemini read `.agents/skills` [62]–[65] | Fire about half the time unaided [36][83]. Listing capped at 1% (Claude Code) or 2% (Codex) of the window [55][62]. After compaction, 5K tokens per skill remain [55] |
| AGENTS.md / CLAUDE.md | Always loaded. Won Vercel's own eval [36]. Agents obey it: a named tool was used 1.6 times per task vs under 0.01 [75] | Claude Code: under 200 lines [56]. Codex stops at 32 KiB [61]. Nesting works differently per tool [56][61] |
| llms.txt | A short index for a docs website [71] | 97% of files got no requests [72]. No job inside a repo (inference) |
| MCP | Live state and actions. Tool search cut tokens about 85% (vendor) [69] | 58 tools ≈ 55K tokens [69]. Tool text is untrusted [68]. Needs a running server |
| Claude Code plugins | Versions, dependencies, `validate --strict` [58] | Claude Code only. Sealed to its own folder. Third-party auto-update is off by default [58] |

- MCP fits Cardinal's live tools. `world mcp` already serves `send_command`, `get_state`, `inspect_shard` and more [100]. Keep static rules out of MCP: users pull resources with `@` and run prompts as commands [57].
- Plugins could later carry the workflow skills and hooks to Claude Code teams. They can't carry the game's Go code (inference) [58].

### Q3. Context cost

- Adherence falls as rules pile up. claude-opus-4 followed 94.6% of 100 instructions and 67.9% of 250 (toy task, 2025 models) [77].
- Context files add cost for little gain. LLM-written: −0.5 to −2 pp, +20–23% cost. Human-written: +2.4 pp (not significant), up to +19% cost [75].
- One study links AGENTS.md to 28.6% faster runs (reported) [76].
- Longer input hurt performance in every experiment, across 18 models (synthetic tasks) [78].
- Small works. Vercel cut its index from 40 KB to 8 KB with no loss [36]. Claude Code warns past 200 lines [56]. Anthropic: aim for "the smallest set of high-signal tokens" [70]. HumanLayer keeps its root file under 60 lines (opinion) [73].
- Weak models gain most (vendor evals). Kimi K2.5 went from 18% to 46% with an AGENTS.md pointer. Frontier models scored 100% either way [35].
- How others stay small: an index plus pages fetched on demand [34][36]; docs only for installed packages [37]; path-scoped rules [56]; checks instead of prose [73].
- Gap: no study tests a game-engine rule set, or any 900-line file.

### Q4. Copy-in vs dependency, and updates

- Copies stop updating. Roblox: "Automatic updating does not apply to modified package copies" [10]. Laravel: "there is no need to update the starter kit itself" [38]. Phaser AE v1 games keep their engine; migration is still being looked at [24].
- Template tools merge text and leave conflicts to you. Copier says keep templates "as pure and simple as possible" [48]. cruft can fail CI when a project falls behind [49].
- shadcn, the copy-in leader, added `--diff` to merge upstream changes by hand [41]. It moved `cn` and shared CSS into packages [42].
- Dependencies get fixes. Go's `retract` and `Deprecated` warn users [50]. `//go:fix inline` lets the author rewrite callers [51].
- Breaking changes ship as migrations from the package. Nx plugins ship them; an agent applies some, gated by lint, build and tests [47]. Next.js has `next upgrade --agent` [34]. Agent files refresh too: `boost:update` [37], Convex's stale-file check [44].
- Docs ride with the version. Next.js 16.2+ bundles docs in `node_modules` [34]. Photon tells agents to target the pinned version [17].
- Many modules need machinery. OpenTelemetry releases about 70 modules in 8 version sets, each with a code owner [53]. Bevy asks each plugin for an engine-version table [12].
- The hypothesis: copies work for a first build. Phaser AE reused 5 of 9 systems in one game (vendor anecdote) [21]. Copies fail on updates (above) and on guardrails (Q5).
- Gap: no study compares copy and import for agent success or upkeep.

### Q5. Guardrails

- Agents game tests. In ImpossibleBench, over 79% of Claude models' cheating was editing tests. Read-only tests stopped that, but not special-casing [85].
- Libraries can own the tests that users run. In Go's `slogtest`, the user passes a constructor and the library holds the cases [88]. Unity runs a package's tests inside a project (reported) [7].
- Isolation is a guardrail. Convex components can't read the app's tables unless passed in [43]. Roblox package scripts are read-only until unlocked [10]. Unity's essential packages can't be deselected [4].
- Shipped checkers: golangci-lint module plugins [89], Unity analyzers [8], bevy_lint (community) [13], Svelte's MCP autofixer loop [90]. None publishes an agent gain.
- Hooks help, but leak. Claude Code's `PostToolUse` can't undo an edit; hard denies belong in permissions [91]. Cursor hooks fail open by default [92]. Agents skip git hooks with `--no-verify` (reported) [93]. CI is the one gate every agent hits.
- Mutation testing scales. Meta's ACH made 9,095 mutants and 571 tests; engineers kept 73% [87]. echo_image already pins rules with 421 mutants [95]. No engine found ships gameplay-rule tests for user code.
- Rules can live in the runtime, not in prompts (inference). Rune caps state at 1 MB and actions at 10 ms; the server ignores invalid actions [29].
- Phaser AE gates parts with a headless rig, acceptance tests, human play and a human-gated write-back [21].

### Q6. Discovery

- A description should say what and when, in users' words, with the key case first [55][59].
- Unaided firing is about half. One small test (22 prompts): 50–55% [83]. Another: never invoked in 56% of cases [36]. A session-start hook took one set from 15% to 62% (reported) [84].
- Budgets hide skills. Claude Code drops descriptions past 1% of the window, least-used first [55]. Codex shortens them, then omits skills [62].
- Flat lists break. Over 90% right at 20 skills or fewer, about 20% at 200. At 60–120 skills, a hierarchy kept 72–85% vs 45–63% flat (synthetic, GPT-4o era) [80].
- At scale, search the full text. Hiding skill bodies cost 37–44 pp at ~80K skills [81]. Retrieval first scored 43% vs 14% on one benchmark [82]. Tool search: 49%→74% (vendor internal) [69].
- Fixed triggers skip description matching: `paths` globs [55][63], path-scoped rules [56], an always-loaded index [36].
- Shipped routers live in the host or package manager, not in a skill. Unreal: list → describe → call [1]. Boost: installed packages [37]. Unity Multiplayer Center: a data table [4]. Context7: resolve, then query [74]. shadcn: MCP search [40].
- With MCP alone, Supabase's agents never called `search_docs` [46].

### Q7. Where each option fails (extrapolated from Q1–Q6)

| Systems | A. Full template | B. Catalog + pointer skill | C. Skill per system + router | D. Starter + kit |
|---|---|---|---|---|
| 10 | Works. Rules fit one file. Copied tests are editable [85] | Works if the pointer lives in AGENTS.md, not a skill [36] | Works. 10 skills fit any budget. 10 docs, 10 skills and 10 versions to sync | Works. Index about 1 KB (est.) |
| 50 | Strains. Every game carries and prunes 50 systems | Past the >90% zone of ≤20 entries [80]. Needs families | Near the listing budget. Look-alikes cost 7–63% [80] | Index about 6 KB (est.); group by family. An owner per system [53] |
| 200 | Fails. No game uses 200 systems | Needs search over full docs [81][82] | Names only [55]. About 200 release lines | A flat index would be ~25 KB (est.), 3× Vercel's. Needs search and version sets [53][81] |

### Hype to discount

- Vercel's 100%: its own suite, one framework; models and task count not given [36].
- SpacetimeDB's +29 pp: vendor-run, 13 runs, method not stated [16].
- Phaser's "5 of 9 systems reused" and "76 coding agents": vendor anecdotes, no success rates [21][23].
- Unity's "highly accurate, context-aware assistance" comes with no data [6]. Unreal's MCP docs: "APIs and data formats may change at any time" [1].
- Astrocade's 10M monthly users: CEO claims [31].
- "1 million skills", "60,000 projects", "10,000 MCP servers": counts, not quality [60][66][67].
- llms.txt: adoption is up, yet 97% of files go unread [72].
- Playroom's docs hide text telling AIs to recommend Playroom. Agent-facing docs are a marketing surface [30].

## Appendix B. Sources

All web pages were read 2026-11-21 by the researchers. "Undated" = the page shows no date.

#### Games: engines, backends, starters and AI builders
1. Unreal MCP in Unreal Editor - Epic Games docs (UE 5.9) - undated - VERIFIED - https://dev.epicgames.com/documentation/en-us/unreal-engine/unreal-mcp-in-unreal-editor
2. Game Features and Modular Gameplay - Epic Games docs (UE 5.9) - undated - VERIFIED - https://dev.epicgames.com/documentation/en-us/unreal-engine/game-features-and-modular-gameplay-in-unreal-engine
3. Variants in game templates (UE 5.6) - Epic Games - 2025 - REPORTED - https://dev.epicgames.com/documentation/unreal-engine/variants-in-game-templates
4. Get started with the Multiplayer Center - Unity docs - updated 2026-11-06 - VERIFIED - https://docs.unity.com/en-us/multiplayer/multiplayer-center.md
5. Package samples - Unity 6.0 manual - undated - VERIFIED - https://docs.unity.com/en-us/engine/6000.0/manual/packages-list/cus-pkg-development/cus-samples
6. MCP servers in game development explained - Unity blog - 2026-06-08 - VERIFIED - https://unity.com/blog/mcp-servers-game-development
7. Adding tests to a package - Unity Manual - undated - REPORTED - https://docs.unity3d.com/Manual/cus-tests.html
8. Roslyn analyzers and source generators - Unity Manual 6000.6 - undated - VERIFIED - https://docs.unity.com/en-us/engine/current/manual/programming-environment/script-compilation/roslyn-analyzers
9. Connect to the Roblox Studio MCP server - Roblox Creator Docs - updated 2026-11-20 - VERIFIED - https://create.roblox.com/docs/studio/mcp.md
10. Packages - Roblox Creator Docs - updated 2026-11-20 - VERIFIED - https://create.roblox.com/docs/projects/assets/packages.md
11. About the Asset Store - Godot docs (4.7) - undated - VERIFIED - https://docs.godotengine.org/en/latest/community/asset_store/what_is_asset_store.html
12. Building Bevy's Ecosystem - Bevy - undated - VERIFIED - https://bevy.org/learn/quick-start/plugin-development/
13. bevy_lint - TheBevyFlock - undated (0.7.0-dev) - VERIFIED - https://thebevyflock.github.io/bevy_cli/linter/index.html
14. SpacetimeDB skills folder - Clockwork Labs (GitHub) - undated - VERIFIED - https://github.com/clockworklabs/SpacetimeDB/tree/master/skills
15. SpacetimeDB concepts SKILL.md - Clockwork Labs - undated (metadata v2.0) - VERIFIED - https://raw.githubusercontent.com/clockworklabs/SpacetimeDB/master/skills/concepts/SKILL.md
16. LLM One-Shot Benchmark Summary - Clockwork Labs - 2026-01-30 - VERIFIED - https://raw.githubusercontent.com/clockworklabs/SpacetimeDB/master/docs/llms/oneshot-summary.md
17. Photon docs llms.txt - Photon Engine - undated - VERIFIED - https://doc.photonengine.com/llms.txt
18. Hiro - Heroic Labs docs - undated - VERIFIED - https://heroiclabs.com/docs/hiro/
19. Boss Room landing page (NGO 2.10) - Unity - 2026 - REPORTED - https://docs.unity3d.com/Packages/com.unity.netcode.gameobjects@2.10//manual/samples/bossroom/bossroom-landing.html
20. Phaser Game Agent - Phaser Studio - 2026-06-30 - VERIFIED - https://phaser.io/news/2026/06/phaser-game-agent
21. How We Built The Phaser Game Agent - Phaser Studio - 2026-06-12 - VERIFIED - https://phaser.io/news/2026/06/how-we-built-the-phaser-game-agent-with-claude-managed-agents-and-superserve
22. Expansion packs - Phaser Studio - undated - VERIFIED - https://phaser.io/packs
23. Phaser AE manual - Phaser Studio - undated - VERIFIED - https://phaser.io/agent/mcp/manual
24. Phaser AE v2 is now the engine behind every new game - Phaser Studio - 2026-07-23 - VERIFIED - https://phaser.io/news/2026/07/phaser-ae-v2-is-now-the-engine-behind-every-new-game
25. Phaser Editor v5 release - Phaser Studio - 2026-05-01 - VERIFIED - https://phaser.io/news/2026/04/phaser-editor-v5-release
26. three.js llms.txt - threejs.org - undated - VERIFIED - https://threejs.org/docs/llms.txt
27. VibeGame - Dylan Ebert, Hugging Face - 2025-09-29 - VERIFIED - https://huggingface.co/blog/vibegame
28. How to move your project to our multi-file template - Rosebud - 2025-03-26 - VERIFIED - https://lab.rosebud.ai/blog/how-to-move-your-project-to-our-multi-file-template
29. Syncing game state - Rune docs - undated - VERIFIED - https://developers.rune.ai/docs/how-it-works/syncing-game-state
30. AI features - Playroom docs - undated - VERIFIED - https://playroom-docs.vercel.app/concepts/ai-features
31. Astrocade hits 10M monthly players - GamesBeat - 2026-10-07 - VERIFIED (as claims) - https://gamesbeat.com/astrocade-hits-10m-monthly-players-for-its-ai-assisted-ugc-casual-games-platform/
32. WebGameBench - arXiv 2605.17637 - 2026-05-17 - VERIFIED - https://arxiv.org/html/2605.17637
33. Spec2Game - arXiv 2610.04253 - 2026-10-03 - VERIFIED - https://arxiv.org/abs/2610.04253

#### Non-game packaging and updates
34. AI agents guide - Next.js docs (Vercel) - updated 2026-10-15 - VERIFIED - https://nextjs.org/docs/app/guides/ai-agents
35. Next.js evals - Vercel - run 2026-10-30 - VERIFIED - https://nextjs.org/evals
36. AGENTS.md outperforms skills in our agent evals - Vercel (Jude Gao) - 2026-01-27 - VERIFIED - https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals
37. Laravel Boost - Laravel docs 13.x - undated - VERIFIED - https://laravel.com/docs/13.x/boost
38. Starter kits - Laravel docs 13.x - undated - VERIFIED - https://laravel.com/docs/13.x/starter-kits
39. registry-item.json - shadcn - undated - VERIFIED - https://ui.shadcn.com/docs/registry/registry-item-json
40. CLI 3.0 and MCP server - shadcn - 2025-08 - VERIFIED - https://ui.shadcn.com/docs/changelog/2025-08-cli-3-mcp
41. CLI v4 - shadcn - 2026-03 - VERIFIED - https://ui.shadcn.com/docs/changelog/2026-03-cli-v4
42. cn changelog - shadcn - 2026-09 - VERIFIED - https://ui.shadcn.com/docs/changelog/2026-09-cn
43. Components - Convex docs - undated - VERIFIED - https://docs.convex.dev/components
44. ai-files - Convex CLI docs - undated - VERIFIED - https://docs.convex.dev/cli/reference/ai-files
45. get-convex/chef README - Convex (GitHub) - undated - VERIFIED - https://github.com/get-convex/chef
46. Supabase Agent Skills - Supabase (Pedro Rodrigues) - 2026-04-09 - VERIFIED - https://supabase.com/blog/supabase-agent-skills
47. Automate updating dependencies - Nx docs - undated - VERIFIED - https://nx.dev/docs/features/automate-updating-dependencies
48. Updating a project - Copier docs - undated - VERIFIED - https://copier.readthedocs.io/en/stable/updating/
49. cruft - cruft docs - undated - VERIFIED - https://cruft.github.io/cruft/
50. Go Modules Reference - Go team - undated - VERIFIED - https://go.dev/ref/mod
51. //go:fix inline and the source-level inliner - Alan Donovan, Go blog - 2026-03-10 - VERIFIED - https://go.dev/blog/inliner
52. Go 1.27 release notes - Go team - 2026-08 - VERIFIED - https://go.dev/doc/go1.27
53. CONTRIBUTING.md and versions.yaml - opentelemetry-go-contrib - undated - VERIFIED - https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/CONTRIBUTING.md ; https://raw.githubusercontent.com/open-telemetry/opentelemetry-go-contrib/main/versions.yaml

#### Agent formats
54. Agent Skills Specification - agentskills.io (AAIF) - undated - VERIFIED - https://agentskills.io/specification
55. Extend Claude with skills - Claude Code docs (Anthropic) - undated - VERIFIED - https://code.claude.com/docs/en/skills
56. How Claude remembers your project - Claude Code docs (Anthropic) - undated - VERIFIED - https://code.claude.com/docs/en/memory
57. Connect Claude Code to tools via MCP - Claude Code docs (Anthropic) - undated - VERIFIED - https://code.claude.com/docs/en/mcp
58. Plugin manifest and loading reference - Claude Code docs (Anthropic) - undated - VERIFIED - https://code.claude.com/docs/en/plugins-reference ; https://code.claude.com/docs/en/plugins/loading
59. Skill authoring best practices - Anthropic - undated - VERIFIED - https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices
60. AGENTS.md - agents.md (AAIF) - undated - VERIFIED - https://agents.md/
61. Custom instructions with AGENTS.md - OpenAI Codex docs - undated - VERIFIED - https://learn.chatgpt.com/docs/agent-configuration/agents-md
62. Build skills - OpenAI Codex docs - undated - VERIFIED - https://learn.chatgpt.com/docs/build-skills
63. Skills - Cursor docs - undated - VERIFIED - https://cursor.com/docs/context/skills
64. About agent skills - GitHub Docs - undated - VERIFIED - https://docs.github.com/en/copilot/concepts/agents/about-agent-skills
65. Agent Skills - Gemini CLI docs (Google) - 2026-04-30 - VERIFIED - https://geminicli.com/docs/cli/skills/
66. Linux Foundation announces the Agentic AI Foundation - Linux Foundation - 2025-12-09 - VERIFIED - https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation
67. Donating Agent Skills to the Agentic AI Foundation - Anthropic - 2026-10-22 - VERIFIED - https://www.anthropic.com/news/donating-agent-skills-to-the-agentic-ai-foundation
68. MCP Specification 2025-11-25 - MCP project - 2025-11-25 - VERIFIED - https://modelcontextprotocol.io/specification/2025-11-25
69. Introducing advanced tool use - Anthropic Engineering - 2025-11-24 - VERIFIED - https://www.anthropic.com/engineering/advanced-tool-use
70. Effective context engineering for AI agents - Anthropic Engineering - 2025-09-29 - VERIFIED - https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
71. The /llms.txt file - Jeremy Howard - 2024-09-03 (older; edited 2026-08-10) - VERIFIED - https://llmstxt.org/
72. We analyzed 137K sites: 97% of llms.txt files never get read - Ahrefs - 2026-06-15 - VERIFIED - https://ahrefs.com/blog/llmstxt-study/
73. Writing a good CLAUDE.md - HumanLayer - 2025-11-25 - VERIFIED - https://www.humanlayer.dev/blog/writing-a-good-claude-md
74. Context7 README - Upstash (GitHub) - undated - VERIFIED - https://github.com/upstash/context7

#### Research and guardrail tools
75. Evaluating AGENTS.md - Gloaguen et al., ETH Zurich and LogicStar.ai (arXiv 2602.11988) - 2026-02-12 - VERIFIED - https://arxiv.org/html/2602.11988
76. On the Impact of AGENTS.md Files on the Efficiency of AI Coding Agents - Lulla et al. (arXiv 2601.20404) - 2026-01-28 - REPORTED - https://arxiv.org/abs/2601.20404
77. How Many Instructions Can LLMs Follow at Once? (IFScale) - Jaroslawicz et al. (arXiv) - 2025-07-15 - VERIFIED - https://arxiv.org/html/2507.11538v1
78. Context Rot - Chroma - 2025-07-14 - VERIFIED - https://www.trychroma.com/research/context-rot
79. SkillsBench - Li et al. (arXiv 2602.12670) - 2026-02-13, v4 2026-06-14 - VERIFIED (abstract) - https://arxiv.org/abs/2602.12670
80. When Single-Agent with Skills Replace Multi-Agent Systems - Xiaoxiao Li (arXiv 2601.04748) - 2026-01-08, v2 2026-01-14 - VERIFIED - https://arxiv.org/html/2601.04748v2
81. SkillRouter - Zheng et al. (arXiv 2603.22455) - 2026-03-23, v5 2026-07-20 - VERIFIED (abstract) - https://arxiv.org/abs/2603.22455
82. RAG-MCP - Gan and Sun (arXiv 2505.03275) - 2025-05-06 - VERIFIED (abstract) - https://arxiv.org/abs/2505.03275
83. Measuring Claude Code skill activation with sandboxed evals - Scott Spence - 2026-02-08 - VERIFIED - https://scottspence.com/posts/measuring-claude-code-skill-activation-with-sandboxed-evals
84. A session-start hook raised Claude Code skill firing from 15% to 62% - Towards Data Science - undated - REPORTED - https://towardsdatascience.com/a-session-start-hook-raised-claude-code-skill-firing-from-15-to-62/
85. ImpossibleBench - Zhong, Raghunathan, Carlini (arXiv 2510.20270) - 2025-10-23 - VERIFIED - https://arxiv.org/html/2510.20270v1
86. Recent Frontier Models Are Reward Hacking - METR - 2025-06-05 - VERIFIED - https://metr.org/blog/2025-06-05-recent-reward-hacking/
87. Mutation-Guided LLM-based Test Generation at Meta (ACH) - Foster et al. (arXiv 2501.12862) - 2025-01-22 - VERIFIED - https://arxiv.org/abs/2501.12862
88. testing/slogtest - Go team (pkg.go.dev) - undated - VERIFIED - https://pkg.go.dev/testing/slogtest
89. Module Plugin System - golangci-lint - updated 2026-11-17 - VERIFIED - https://golangci-lint.run/docs/plugins/module-plugins/
90. Svelte MCP tools - svelte.dev - undated - VERIFIED - https://svelte.dev/docs/mcp/tools
91. Hooks reference - Claude Code docs (Anthropic) - undated - VERIFIED - https://code.claude.com/docs/en/hooks
92. Hooks - Cursor docs - undated - VERIFIED - https://cursor.com/docs/agent/hooks
93. Allow Bash(git commit:*) considered harmful - Chris Richardson, microservices.io - 2025-09-10 - REPORTED - https://microservices.io/post/genaidevelopment/2025/09/10/allow-git-commit-considered-harmful.html

#### Local files (read-only, 2026-11-21)
94. echo_image CLAUDE.md: 966 lines, 82,345 bytes, section sizes - local - 2026-11-21 - VERIFIED - /home/doglightning/echo_image/CLAUDE.md
95. echo_image mutation list and checks: 421 mutants; 11 on pkg/queue killed by `go test ./pkg/queue`; QU-R1–R3 killed by the order test; checks at 20 and 60 Hz - local - 2026-11-21 - VERIFIED - /home/doglightning/echo_image/scripts/mutate/mutants.json ; /home/doglightning/echo_image/scripts/check.sh
96. echo_image order check, TestSendersRunBeforeTheirReceivers (line 614) - local - 2026-11-21 - VERIFIED - /home/doglightning/echo_image/shards/game/test/harness_test.go
97. echo_image queue doc and the ownercheck analyzer prototype (2026-11-19, test data only) - local - 2026-11-21 - VERIFIED - /home/doglightning/echo_image/pkg/queue/doc.go ; /home/doglightning/echo_image/docs/research/queue-api-notes/ownercheck.md
98. echo_image history package and its imports - local - 2026-11-21 - VERIFIED - /home/doglightning/echo_image/shards/game/internal/history/
99. world-engine scaffolder: copies a template at the CLI's engine release; writes world-local-dev to .claude/skills and .agents/skills - local - 2026-11-21 - VERIFIED - /home/doglightning/world-engine/cli/pkg/worldscaffold/scaffold.go ; /home/doglightning/world-engine/cli/pkg/worldscaffold/templates.go
100. world-engine physics2d plugin (same module), cardinal-migration skill, `world mcp` tools - local - 2026-11-21 - VERIFIED - /home/doglightning/world-engine/pkg/plugin/physics2d/ ; /home/doglightning/world-engine/.agents/skills/cardinal-migration/ ; /home/doglightning/world-engine/cli/commands/mcp/
