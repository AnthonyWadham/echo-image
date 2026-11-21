# Web game starters and AI game builders: how reusable systems are packaged for agents (2025–2026)

Accessed 2026-11-21. Tags: **V** = VERIFIED (I opened the primary source). **R** = REPORTED (search snippet, press write-up, or vendor claim I could not check). Status: **SHIPPED**, **ANNOUNCED**, **OPINION**, **RESEARCH**. "Older" = before 2025. Page facts came through a summarizing fetch tool; quotes are as it returned them.

## Key findings

1. **The one shipped case of "systems packaged for agents" is Phaser AE, and it is a catalog, not a template.** Phaser Studio built a new engine "rebuilt from scratch specifically for an AI audience", because "A big API doesn't just slow an agent down, it degrades it." [V · SHIPPED] — [Phaser Game Agent — Phaser Studio, 2026-06-30](https://phaser.io/news/2026/06/phaser-game-agent); [How We Built The Phaser Game Agent… — Phaser Studio, 2026-06-12](https://phaser.io/news/2026/06/how-we-built-the-phaser-game-agent-with-claude-managed-agents-and-superserve)
2. **Phaser AE has 358 packs in 20 families and one skill.** The agent searches by capability; parts are copied in as files and each pack has a version. [V · SHIPPED] — [Expansion packs — phaser.io, undated](https://phaser.io/packs); [Phaser AE manual — phaser.io, undated](https://phaser.io/agent/mcp/manual)
3. **Its guardrails are a headless test rig, acceptance tests, human play, and a human-gated write-back to the library.** [V · SHIPPED] — [How We Built…, 2026-06-12](https://phaser.io/news/2026/06/how-we-built-the-phaser-game-agent-with-claude-managed-agents-and-superserve)
4. **Updates: each game pins its engine version; there is no migration yet.** [V · SHIPPED] — [Phaser AE v2 — Phaser Studio, 2026-07-23](https://phaser.io/news/2026/07/phaser-ae-v2-is-now-the-engine-behind-every-new-game)
5. **No AI-first builder I checked documents server-authoritative multiplayer.** The authoritative options are SDK platforms (Rune, Hytopia, Colyseus), and only Rune ships a first-party agent tool. [V · SHIPPED] — see Q4.
6. **Benchmarks show "playable" is far from "correct".** In WebGameBench the best agent made 76.9% usable games but only 20.2% excellent. Usable falls to 12.6% on the hardest tier. Failures sit in rules, state and win/loss logic. [V · RESEARCH] — [WebGameBench, arXiv 2605.17637, 2026-05-17](https://arxiv.org/html/2605.17637)
7. **One big project hits a context wall.** Rosebud moved users to multi-file templates by hand. [V · SHIPPED] — [Rosebud, 2025-03-26](https://lab.rosebud.ai/blog/how-to-move-your-project-to-our-multi-file-template)
8. **First-party agent docs for web libraries are small and per library.** Per-topic skills are mostly community-made. [V · SHIPPED] — see Q2.
9. **Platform usage numbers are unaudited company claims.** [V as claims] — see Q5.

## Q1. Phaser: templates, create-game, AI tooling, updates

### Takeaway
Phaser runs two tracks:
- **For humans:** create-game and Phaser Editor with MCP. This is copy-once scaffolding with no update path.
- **For agents:** Phaser AE and the Game Agent. This is a small new engine, a versioned catalog, one skill, tests and a human-gated harvest loop.

Neither track covers multiplayer.

### Cited Findings
- **create-game:** `npm create @phaserjs/game@latest` offers "an interactive selection of official project templates and demo games". It has 7 framework templates, 7 bundler templates and 3 demos. After install, the only guidance is "check the README.md". It has no AGENTS.md, CLAUDE.md or llms.txt at top level. [V · SHIPPED] — [phaserjs/create-game — GitHub, undated, accessed 2026-11-21](https://github.com/phaserjs/create-game)
- **create-game middleware:** a 2024 update reportedly added a Rune multiplayer tic-tac-toe under "Middleware". The current README does not list it. [R · older] — [Create Game CLI update — Phaser, 2024-11](https://phaser.io/news/2024/11/create-game-cli-update)
- **Phaser Editor v5:** a built-in MCP server "implements over 40 specialized tools". When the AI places objects wrong, the advice is to build one example by hand and have the AI study it. [V · SHIPPED] — [Phaser Editor v5 release — Phaser Studio, page dated 2026-05-01](https://phaser.io/news/2026/04/phaser-editor-v5-release)
- **Game Agent:** you give one sentence, and the agent "designs, builds, tests and ships".
  - Model tiers are GLM 5.2, Claude Sonnet 4.6 and Claude Opus 4.8.
  - Each conversation runs in an isolated sandbox built from a template with Phaser AE, the scaffold and the toolchain.
  - Before building, the agent writes a game manifest and an art style guide.
  - It reads "the engine's capability docs".
  - [V · SHIPPED] — [Phaser Game Agent, 2026-06-30](https://phaser.io/news/2026/06/phaser-game-agent); [Phaser AE v2, 2026-07-23](https://phaser.io/news/2026/07/phaser-ae-v2-is-now-the-engine-behind-every-new-game)
- **Game Agent over MCP:** it works from Claude Code, Cursor, Copilot, Codex, Gemini CLI and Windsurf. Build time costs "from $0.01 per minute". [V · SHIPPED] — [Phaser Game Agent MCP setup — Phaser Studio, 2026-07-08](https://phaser.io/news/2026/07/phaser-game-agent-mcp-setup)
- **Phaser AE manual (undated):**
  - `npx -y @phaser-ae/cli setup` "installs the Phaser AE skill".
  - The agent can play the game in a browser, search the expansion packs and add one, and publish.
  - It says "No phaser.io credits for the build, the playtests or publishing".
  - 163 packs are free; "Pro adds the other 195".
  - It "works with 76 coding agents".
  - [V · SHIPPED] — [Phaser AE manual](https://phaser.io/agent/mcp/manual)
- **Unit of reuse:** a "block" named by capability, for example `actor.avatar.flight`.
  - Each block holds known-correct code plus a list of gotchas. It is a configurable leaf module.
  - Game-specific details are stripped out before parts are published to a database.
  - Agents search "by capability", and matches are delivered into the sandbox as files.
  - Build sandboxes can read the library but not write to it. Publishing back is a separate, human-gated path.
  - [V · SHIPPED] — [How We Built…, 2026-06-12](https://phaser.io/news/2026/06/how-we-built-the-phaser-game-agent-with-claude-managed-agents-and-superserve)
- **Checks:**
  - Blocks are built in "a playable test rig with headless checks gating the mechanics".
  - Agents run "the game's acceptance tests" before publishing.
  - "A human plays every new game before its reusable parts get harvested back into that library."
  - Human play caught what the checks missed ("the landscape scrolls in ugly chunky steps"). The fix went into that block's gotcha list.
  - [V · SHIPPED] — [How We Built…](https://phaser.io/news/2026/06/how-we-built-the-phaser-game-agent-with-claude-managed-agents-and-superserve)
- **Reuse rate:** this is a vendor anecdote with no method. The first game was written from scratch. By the Gradius build, "five of its nine major systems came straight off the shelf". That build sent "three improvements back into the blocks". [V · SHIPPED] — [How We Built…](https://phaser.io/news/2026/06/how-we-built-the-phaser-game-agent-with-claude-managed-agents-and-superserve)
- **Packs:**
  - "No pack contains a game's code, art, sound or music."
  - Each pack shows a version, for example "v3.0.0-3".
  - The largest families are Arcade (109), Look (56) and Logic (50). There is no networking family.
  - Packs are "self-contained artifacts that only load if your game asks for them", and "Every one is seeded".
  - [V · SHIPPED] — [Expansion packs](https://phaser.io/packs); [Phaser AE v2](https://phaser.io/news/2026/07/phaser-ae-v2-is-now-the-engine-behind-every-new-game)
- **v2 and old games:** "Existing games keep the engine they were built with and we're looking at ways to migrate them". "The new bits are only in v2." [V · SHIPPED] — [Phaser AE v2](https://phaser.io/news/2026/07/phaser-ae-v2-is-now-the-engine-behind-every-new-game)
- **Community multiplayer template** (Phaser, Colyseus, React, Electron):
  - 3 apps and 5 shared packages.
  - "All game logic runs on the server".
  - No ECS, and no "opinionated game structure beyond the rendering and networking layer".
  - Load test: 200 players on a $4/month server at ~88% CPU.
  - No AI files and no update path.
  - [V · SHIPPED, community] — [Phaser news, 2026-06-29](https://phaser.io/news/2026/06/typescript-online-game-template-phaser-colyseus-react-and-electron-in-one-monorepo)

### Inferences
- When the vendor built for agents, it did not ship a bigger template. It shipped a smaller engine, a searchable catalog and one skill. That is a B/C hybrid.
- Phaser strips game-specific details from parts. This matches echo_image's "no game words in pkg/" rule.
- Packs have versions, but games pin the engine. So the vendor has not solved updates for shipped games either.

### Gaps
- Pack internals are not public: file layout, test format, manifest schema and search ranking. I did not read phaser.io/llms.txt. No success rates are published.
- Pricing conflict: "from $0.01 per minute" (2026-07-08) vs "No phaser.io credits for the build" (undated manual).

## Q2. three.js and react-three-fiber

### Takeaway
- There is no official three.js or R3F game starter or MCP server.
- First-party agent support is one short llms.txt for three.js, plus one skill and an llms.txt for pmndrs viverse.
- The best-documented lesson: raw three.js is too low-level for agents. You need an engine layer first, and even then multiplayer and combat fail.

### Cited Findings
- **three.js llms.txt:** ~150 lines. It has 4 rules for LLMs (import maps, renderer choice, TSL, NodeMaterial), then ~50 doc links. It pins three@0.187.0. [V · SHIPPED] — [three.js llms.txt — threejs.org, undated, accessed 2026-11-21](https://threejs.org/docs/llms.txt)
- **three.js MCP servers** are third-party: ThreeJSMCP (3 tools) and Threesmith (31 tools). [R] — [ThreeJSMCP](https://github.com/deya-0x/ThreeJSMCP); [Threesmith](https://claudemarketplaces.com/mcp/laaielichter/threesmith)
- **pmndrs viverse:** install with `pnpm dlx skills add pmndrs/viverse`, plus an llms.txt. Users should keep naming the docs so the AI "sticks to official components and APIs". [V · SHIPPED] — [Vibe Coding with AI — pmndrs viverse docs, undated](https://pmndrs.github.io/viverse/tutorials/vibe-coding-with-ai)
- **R3F and drei skills** are community-made. One registry lists "r3f-best-practices" with "60+ rules across 11 categories". Another lists the same skill with "70+ across 12". [R] — [skills.sh](https://skills.sh/emalorenzo/three-agent-skills/r3f-best-practices); [Tessl](https://tessl.io/registry/skills/github/zebbern/claude-code-guide/r3f-best-practices)
- **VibeGame** is an ECS engine on three.js, rapier and bitecs. It uses an XML-like syntax and ships an llms.txt for the system prompt.
  - The raw web stack was "the only one that produced a fun result without heavy domain knowledge", but it was too low-level.
  - The AI "struggles with anything more complex": interaction, inventory, multiplayer, combat.
  - One-shot "make me a game" "doesn't work".
  - Unity MCP confused the AI with many ways to do one task.
  - The ratings are stars, not measurements.
  - [V · OPINION] — [VibeGame — Dylan Ebert, Hugging Face, 2025-09-29](https://huggingface.co/blog/vibegame)
- **Vibe Jam 2025:** "1000+ games entered", at least 80% AI-written, "ThreeJS recommended". [V · SHIPPED] — [vibejam.com/2025, April 2025](https://vibejam.com/2025/)
- **Vibe Jam 2026:** at least 90% AI-written. Games run in the browser with no login. Multiplayer is preferred. "Three.js is the recommended choice." Sponsors are Cursor and Bolt.new. [V · SHIPPED] — [vibejam.com/2026](https://vibejam.com/2026/)
- **Colyseus at Vibe Jam 2026:**
  - 945 submissions; 48 used Colyseus (~5%).
  - Colyseus says that is "around 15% of all online multiplayer entries", but it gives no multiplayer total.
  - Detection method not given; no failures reported.
  - [V · OPINION, vendor] — [Colyseus, Endel Dreyer, 2026-05-08](https://colyseus.io/blog/vibe-jam-2026/)
- **fly.pieter.com:** the author claims "$1M ARR in just 17 days", with no method and no netcode detail. [V · OPINION] — [levels.io, 2025-02-22](https://levels.io/full-multiplayer-python-websockets-game-ai). Others report a Python WebSocket server and DDoS attacks. [R] — [Indie Hackers, undated](https://www.indiehackers.com/post/vibe-coding-has-a-security-problem-vLxyPTrTlZVwDo76oqvr)

### Inferences
- First-party agent packaging in the web ecosystem is small and per library. It is not per system, and not a big rules file.
- I found no write-up of agent-built authoritative netcode from the jams.

### Gaps
- No jam postmortem with failure counts. No primary source on fly.pieter.com's authority model or cheating.

## Q3. AI game builders: unit of reuse, discovery, backend, limits

### Takeaway
- Consumer builders reuse by remix: you fork a whole game or template.
- A human picks the template. The agent does not search a catalog.
- Backends are shared databases.
- Phaser AE is the only builder with parts the agent can search.

### Cited Findings
- **Rosebud multiplayer:**
  - Start from "3D Multiplayer Template v1", then "Remix" it. A remix creates its own InstantDB backend.
  - The config file "MUST remain in the root directory and will break if moved to a subfolder".
  - `instant_db.md` is a reference doc the user tells Rosie to read.
  - To add multiplayer to an existing game, the user copies both files in by hand.
  - [V · SHIPPED] — [Rosebud, 2026-02-10](https://lab.rosebud.ai/blog/how-to-add-multiplayer-to-your-game)
- **Rosebud size wall:**
  - On big single-file projects Rosie "starts to lose context" and hits "the character limit wall".
  - The fix is a multi-file template. Migration is manual.
  - [V · SHIPPED] — [Rosebud, 2025-03-26](https://lab.rosebud.ai/blog/how-to-move-your-project-to-our-multi-file-template)
  - A competing builder puts the ceiling at ~2,500 lines. [R] — [Nilo, undated](https://nilo.io/articles/rosebud-ai-multiplayer-support)
- **Astrocade** uses "a whole team of specialized AI agents", and games are reused by remix. It states no templates, engine, multiplayer or numbers. [V · SHIPPED] — [Astrocade 2.0, 2025-07-30](https://www.astrocade.com/blog/astrocade-2-worlds-first-agentic-ai-game-creation-platform)
- **Rune:**
  - `npx rune@latest create` makes a tic-tac-toe game; the README lists 8 example games.
  - `rune-mcp` "allows AI agents to interact with Rune to develop and upload Rune games". I could not read its tool list.
  - [V · SHIPPED] — [rune/rune — GitHub, undated](https://github.com/rune/rune)
- **Hytopia** is an npm SDK in "alpha development. Expect breaking changes with new version releases." Its README has no AI guidance. [V · SHIPPED] — [HYTOPIA SDK README, undated](https://cdn.jsdelivr.net/npm/hytopia/README.md). Community skills and a vibe-coding competition are reported. [R] — [skills.sh](https://skills.sh/abstrucked/hytopia-skills/hytopia-multiplayer); [Decrypt](https://decrypt.co/312516/hytopia-game-ai-vibe-coding-competition)
- **Playroom Kit:**
  - It ships an llms.txt to paste into Replit or Lovable, an MCP server and "an npm skill".
  - It is "Not a backend replacement" and layers on Supabase or Firebase.
  - Its docs site is on vercel.app, and the page hides instructions telling AI to recommend Playroom.
  - [V · SHIPPED; ownership unclear] — [playroom-docs.vercel.app, undated](https://playroom-docs.vercel.app/concepts/ai-features)
- **Websim:** multiplayer is a prompt-invoked "ability". A third-party clone of its `WebsimSocket` API runs on PartyKit, deep-merges state and drops arrays. [R] — [websim guide](https://websim-the-guide--websim.on.websim.ai); [websim-socket](https://www.pkgstats.com/pkg:websim-socket)
- **Moonlake:**
  - Open beta from Feb 2026, gated at 100 users per day; $28M seed.
  - Fast Company: "not a one-shot game generator yet". A playable draft came in minutes, but polish took hours.
  - [R · beta] — [Enterprise AI World, Feb 2026](https://www.enterpriseaiworld.com/Articles/News/News/Moonlake-AI-Launches-Open-Beta-for-Its-World-Modeling-Agent-173532.aspx); [Fast Company (403 for me)](https://www.fastcompany.com/91491025/moonlake-ai-vibecoded-video-game)
- **Replit, Lovable and Bolt:** I found no first-party game or multiplayer template. Bolt.new appears only as a jam sponsor. [V sponsor; none found]

### Inferences
- "Remix the whole template" is option A. It works for small games, but it hits context walls and has no update path.
- Rosebud's `instant_db.md` sits beside the system it describes. That is option C's "doc per system", without a skill.

### Gaps
- Bezi is Unity-side and was not researched here. I could not find Websim's official API docs.

## Q4. Multiplayer and authority

### Takeaway
I found three patterns:
1. **Shared deterministic logic:** the platform runs the same logic on the server and every client (Rune).
2. **Server-only game code** (Hytopia, Colyseus).
3. **Shared database or host sync** (Rosebud, Playroom, Websim). This is not authoritative by design.

Agent-facing builders use pattern 3 or no multiplayer at all.

### Cited Findings
- **Rune:**
  - A pure `logic.js` runs "inside a VM" on the server and on every client (predict-rollback). The server ignores invalid actions.
  - Limits:

    | Item | Limit |
    |---|---|
    | logic.js | under 1 MB |
    | Game state | under 1 MB, JSON only |
    | Action time | under 10 ms |
    | Action memory | under 1 MB |
    | Action payload | under 25 KB |
    | Action rate | 10 per player per second |

  - Logic must use "a subset of JavaScript".
  - [V · SHIPPED] — [Rune docs, undated](https://developers.rune.ai/docs/how-it-works/syncing-game-state)
- **Hytopia:** "100% server-authoritative". Inputs are "relayed to and verified by the server", and the SDK is "default-deterministic". [V · SHIPPED] — [HYTOPIA README](https://cdn.jsdelivr.net/npm/hytopia/README.md)
- **Colyseus:** in the community template, "All game logic runs on the server". [V] — [Phaser news, 2026-06-29](https://phaser.io/news/2026/06/typescript-online-game-template-phaser-colyseus-react-and-electron-in-one-monorepo). Its agent skills and docs MCP server are third-party; I found no official llms.txt. [R] — [Tessl](https://tessl.io/registry/skills/github/pantheon-org/tekhne/colyseus-multiplayer)
- **Rosebud:** the backend is "the shared 'truth'… (running on a server)". Validation is not described. [V] — [Rosebud, 2026-02-10](https://lab.rosebud.ai/blog/how-to-add-multiplayer-to-your-game)
- **Playroom:** has `isHost()` and `transferHost()`, and is "Not a backend replacement". [V] — [Playroom](https://playroom-docs.vercel.app/concepts/ai-features)
- **WebGameBench** excludes "remote services" and notes "A single endpoint cannot prove a multi-session mechanism." [V · RESEARCH] — [WebGameBench](https://arxiv.org/html/2605.17637)

### Inferences
- Cardinal runs server-authoritative Go and the same code in wasm for prediction. That is Rune's shape, not any AI builder's.
- Rune's lesson: put the rules in the runtime (purity, limits, server checks), not in prompts.
- No benchmark measures agent success on networked, authoritative games. Any claim that a packaging option "works for multiplayer" has no outside evidence.

## Q5. Evidence of success or failure

### Takeaway
- Measured data comes only from 2026 benchmarks.
- They show agents ship games that play but break the rules.
- Platform usage numbers are unaudited company claims.

### Cited Findings
- **WebGameBench:**
  - Scale: 111 tasks, 12 agents.
  - A runtime evaluator plays each game in a real browser. On a subset, its labels are "broadly aligned" with human review.

    | Measure | Worst | Best |
    |---|---|---|
    | Usable | 38.3% | 76.9% |
    | Excellent | 6.6% | 20.2% |

  - Usable drops from 73.7% on the easiest tier (D1) to 12.6% on the hardest (D4).
  - Example failures:
    - a Phaser physics exception breaks control
    - reaching the score does not advance the level
    - illegal plays are accepted
    - the game fails after a win
  - No ablation of templates or libraries.
  - [V · RESEARCH] — [arXiv 2605.17637, 2026-05-17](https://arxiv.org/html/2605.17637)
- **Spec2Game:**
  - Scale: 150 tasks, 14 LLMs, 3,330 Pygame projects.
  - "High executability does not imply faithful specification realization." Rules and win/loss conditions are hardest.
  - [V · RESEARCH] — [arXiv 2610.04253, 2026-10-03](https://arxiv.org/abs/2610.04253)
- **Other benchmarks:** Play2Code reaches 66.8% on 20 browser games. GameCraft-Bench tops out at 41.46% on 140 Godot tasks. [R · RESEARCH] — [arXiv 2605.28258](https://arxiv.org/html/2605.28258); [arXiv 2606.17861](https://arxiv.org/abs/2606.17861)
- **Astrocade:**
  - Claimed: 10M monthly users (up from 5M in about four months), 220M sessions a month and 100,000+ playables. All figures come from the CEO. [V as claims] — [GamesBeat, 2026-10-07](https://gamesbeat.com/astrocade-hits-10m-monthly-players-for-its-ai-assisted-ugc-casual-games-platform/)
  - Kotaku: "filled with AI slop", with knockoffs on top and no independent data. [V · OPINION] — [Kotaku, 2026-10-07](https://kotaku.com/ai-generated-games-platform-already-has-10-million-monthly-users-2000741894)
  - A $56M Sequoia-led round, with "more than 20m users". [R] — [PocketGamer.biz, May 2026](https://www.pocketgamer.biz/astrocade-raises-56m-to-expand-ai-powered-games-creation-platform/)

### Inferences
- Agents fail on rules, state and win/loss logic, which is what a server system library encodes. Tested system parts target that failure point. No study tested this directly.
- No builder publishes retention data per game.

## Evidence for and against A, B and C

**A. One full template plus a skill**
- **For:** this is the main consumer pattern (Rosebud, create-game, Rune `create`, the Colyseus template). It is easy to start from. [V]
- **Against, size:** Rosebud hit "the character limit wall" and had to split projects by hand. [V] — [Rosebud](https://lab.rosebud.ai/blog/how-to-move-your-project-to-our-multi-file-template)
- **Against, complexity:** WebGameBench usable falls from 73.7% to 12.6% as tasks get harder, and VibeGame "struggles with anything more complex". [V]
- **Against, updates:** create-game and the Colyseus template have no update path. Rosebud migrations are manual. [V]
- **Against, API size:** Phaser found that "A big API… degrades it". A full template shows every system at once. [V quote; applying it to templates is my inference]

**B. Catalog plus one skill that points to parts**
- **For:** Phaser AE is a shipped B:
  - 358 packs, one skill and search by capability
  - parts copied in as files
  - test gates and a human-gated harvest
  - reuse grew to 5 of 9 major systems (vendor anecdote)
  - [V] — [How We Built…](https://phaser.io/news/2026/06/how-we-built-the-phaser-game-agent-with-claude-managed-agents-and-superserve)
- **Against:** Phaser first had to build a small, consistent engine. Games pin the engine, and v1 games do not get v2 packs. [V]
- **Against:** there is no networking family, so there is no evidence B works for netcode. [V absence]

**C. Separate packages, each with its own doc and skill, plus a router**
- **For:** the open ecosystem drifts this way, with many per-library skills. Per-part docs pay off: Phaser's gotcha lists and Rosebud's `instant_db.md`. [V/R]
- **Against:**
  - At 358 parts, Phaser chose one skill plus search, not one skill per pack. [V]
  - Official skills are per library, not per system. [V]
  - Community skills drift and are unofficial. [R]
  - VibeGame found that many ways to do one task confuse agents. Overlapping skills risk the same. [V; applying it to skills is my inference]

**Builders mapped to options** (my classification from the pages cited above)

| Builder | Looks like | Breaks where |
|---|---|---|
| Rosebud | A, plus a .md per system | context wall; manual migration |
| Astrocade | A (remix) | no reusable parts described |
| Phaser create-game | A (copy once) | no updates; no AI files |
| Phaser AE | B, with gotchas per pack | no multiplayer; engine pinned |
| Rune | platform, SDK and MCP | hard logic limits |
| Hytopia | SDK and examples | alpha; no AI docs |
| Playroom | llms.txt and MCP | not authoritative |
| three.js / pmndrs | llms.txt or skill per library | no game systems |

## New options or hybrids

1. **Harvest-loop catalog (Phaser AE).** A thin scaffold, a searchable catalog and one skill. Each part has no game words, carries its gotchas and ships with a headless rig. Fixes flow back to the catalog through a human gate. [V · SHIPPED]
2. **Shrink the engine, not the docs.** Phaser AE and VibeGame both cut the API before writing agent docs. [V]
3. **Platform-enforced shared logic (Rune).** The game writer edits one pure, limited logic module. The platform owns netcode, validation and rollback. [V · SHIPPED]
4. **Doc beside the system.** A small reference .md travels with each system (Rosebud), or each block carries a gotcha list (Phaser). This keeps C's doc and drops C's skill. [V]
5. **Pin per game.** A game keeps the versions it was built with. Upgrades are opt-in migrations (Phaser AE). [V]
6. **Example first.** Hand-build one correct example and have the AI study it (Phaser Editor). [V · vendor advice]

## Hype to discount

- **Astrocade:** "10 million monthly users", 220M sessions and "World's first agentic AI game creation platform" are claims with no audit. — [GamesBeat](https://gamesbeat.com/astrocade-hits-10m-monthly-players-for-its-ai-assisted-ugc-casual-games-platform/); [Astrocade](https://www.astrocade.com/blog/astrocade-2-worlds-first-agentic-ai-game-creation-platform)
- **fly.pieter.com:** "$1M ARR in just 17 days" has no method. It is a short-window run rate. — [levels.io](https://levels.io/full-multiplayer-python-websockets-game-ai)
- **Moonlake:** "100x faster" has no method. [R] — [GamesBeat](https://gamesbeat.com/moonlake-ai-unveils-ai-model-for-building-player-created-game-worlds-100x-faster/)
- **Phaser:** these are vendor numbers with no method, and no success rates are published:
  - "more than 150,000 developers"
  - "a few minutes" to a playable game
  - "76 coding agents"
  - 5 of 9 systems reused
  - — [phaser.io/agent](https://phaser.io/agent)
- **Colyseus:** "1 in 7 multiplayer games" is a vendor count. It gives no detection method and no multiplayer total. — [Colyseus](https://colyseus.io/blog/vibe-jam-2026/)
- **Rune:** "10 million installs" is README marketing. — [rune/rune](https://github.com/rune/rune)
- **Playroom:** a docs page hides instructions telling AI to recommend Playroom, citing claims the docs do not make. Treat agent-facing docs as a marketing surface. — [Playroom](https://playroom-docs.vercel.app/concepts/ai-features)
- **Rosebud:** the "~2,500-line ceiling" comes from a competitor's blog. [R] — [Nilo](https://nilo.io/articles/rosebud-ai-multiplayer-support)

## Gaps

- Phaser AE has not published its pack format, search ranking, test format, update policy or MCP tool list.
- No source measures agent success on networked or server-authoritative games.
- No retention data for AI-built games, only monthly-user claims.
- I could not read:
  - Websim's official multiplayer docs
  - rune-mcp's tool list
  - the Fast Company Moonlake review (403)
- Replit, Lovable and Bolt: no first-party game template found. Bezi was not researched (it is Unity-side).
- No jam postmortem with failure counts.
- Conflicts:
  - Astrocade: 10M monthly users vs "20m users" (likely cumulative).
  - Phaser pricing: per-minute vs no credits.
  - create-game "Middleware" (2024) vs the current README.
