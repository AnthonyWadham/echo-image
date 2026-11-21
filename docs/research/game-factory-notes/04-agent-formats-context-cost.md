# Agent-side packaging formats and context cost (2025–2026)

Researcher 04. All sources accessed 2026-11-21.
Tags: VERIFIED = I opened the primary source. REPORTED = second-hand (search snippet, summary, or a vendor claim I could not check). Then SHIPPED / ANNOUNCED / OPINION / RESEARCH. "Inference" = my reasoning, not a source claim.

## Key findings

Detail and full tags are in Q1–Q7.

1. **Skills often fail to trigger.** Vercel: the skill was never invoked in 56% of cases; scores were skill 53% = no docs 53%, and an always-loaded 8 KB index 100% ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals)). Unaided triggering ran 50–55% in another test ([S28](https://scottspence.com/posts/measuring-claude-code-skill-activation-with-sandboxed-evals)). [VERIFIED|OPINION]
2. **Listing budgets cap the visible skills.** Claude Code: 1% of the context window. Codex: 2% of the window, or 8,000 chars. Both shorten or drop descriptions past the budget. [VERIFIED|SHIPPED] ([S3](https://code.claude.com/docs/en/skills), [S12](https://learn.chatgpt.com/docs/build-skills))
3. **Context files add cost and little success.** LLM-written files: −0.5 to −2 pp success, +20–23% cost. Developer-written: +2.4 pp (not significant), up to +19% cost. [VERIFIED|RESEARCH] ([S23](https://arxiv.org/html/2602.11988))
4. **Size targets.** Claude Code aims for under 200 lines per CLAUDE.md. Codex reads 32 KiB by default ([S4](https://code.claude.com/docs/en/memory), [S11](https://learn.chatgpt.com/docs/agent-configuration/agents-md)). echo_image `CLAUDE.md` is 966 lines and 82,345 bytes (measured 2026-11-21). That is 4.8× and 2.5× those limits. (Inference)
5. **Adherence falls as instructions pile up.** claude-opus-4 scores 94.6% at 100 instructions and 67.9% at 250. [VERIFIED|RESEARCH] ([S25](https://arxiv.org/html/2507.11538v1))
6. **Focused skills win.** Skills with ≤3 modules beat exhaustive bundles; curated skills add +16.6 pp. [VERIFIED|RESEARCH] ([S27](https://arxiv.org/abs/2602.12670))
7. **Agent Skills is the cross-tool format.** Codex, Cursor, Copilot and Gemini CLI read `.agents/skills/`. Claude Code's docs list `.claude/skills/`. [VERIFIED|SHIPPED] ([S12](https://learn.chatgpt.com/docs/build-skills)–[S15](https://geminicli.com/docs/cli/skills/), [S3](https://code.claude.com/docs/en/skills))
8. **Fixed triggers exist.** File-path globs (`paths`), rules files that load when matching files are touched, and hooks. [VERIFIED|SHIPPED] ([S3](https://code.claude.com/docs/en/skills), [S4](https://code.claude.com/docs/en/memory), [S13](https://cursor.com/docs/context/skills))
9. **MCP tools cost a lot of context.** 58 tools take ~55K tokens. Claude Code defers them by default. [VERIFIED|SHIPPED] ([S19](https://www.anthropic.com/engineering/advanced-tool-use), [S5](https://code.claude.com/docs/en/mcp))
10. **Plugins are versioned but work only in Claude Code.** Each is sealed to its own folder, and third-party auto-update is off by default. [VERIFIED|SHIPPED] ([S6](https://code.claude.com/docs/en/plugins-reference), [S7](https://code.claude.com/docs/en/plugins/loading))
11. **llms.txt is a web-docs index.** 97% of files got zero requests. [VERIFIED|RESEARCH] ([S32](https://ahrefs.com/blog/llmstxt-study/))

### Source register

| ID | Title | Publisher / author | Date |
|---|---|---|---|
| S1 | [Agent Skills Specification](https://agentskills.io/specification) | agentskills.io (AAIF) | undated |
| S3 | [Extend Claude with skills](https://code.claude.com/docs/en/skills) | Anthropic, Claude Code docs | undated (cites v2.1.2xx) |
| S4 | [How Claude remembers your project](https://code.claude.com/docs/en/memory) | Anthropic | undated (cites v2.1.309) |
| S5 | [Connect Claude Code to tools via MCP](https://code.claude.com/docs/en/mcp) | Anthropic | undated |
| S6 | [Plugin manifest reference](https://code.claude.com/docs/en/plugins-reference) | Anthropic | undated |
| S7 | [Plugin loading reference](https://code.claude.com/docs/en/plugins/loading) | Anthropic | undated |
| S8 | [Create a marketplace](https://code.claude.com/docs/en/plugin-marketplaces) | Anthropic | undated |
| S9 | [Skill authoring best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices) | Anthropic | undated |
| S10 | [AGENTS.md](https://agents.md/) | AAIF / LF Projects | undated |
| S11 | [Custom instructions with AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md) | OpenAI (Codex) | undated |
| S12 | [Build skills](https://learn.chatgpt.com/docs/build-skills) | OpenAI (Codex) | undated |
| S13 | [Skills](https://cursor.com/docs/context/skills) | Cursor | undated (mentions 2.4) |
| S14 | [About agent skills](https://docs.github.com/en/copilot/concepts/agents/about-agent-skills) | GitHub Docs | undated |
| S15 | [Agent Skills](https://geminicli.com/docs/cli/skills/) | Google, Gemini CLI | 2026-04-30 |
| S16 | [LF announces the Agentic AI Foundation](https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation) | Linux Foundation | 2025-12-09 |
| S17 | [Donating Agent Skills to the AAIF](https://www.anthropic.com/news/donating-agent-skills-to-the-agentic-ai-foundation) | Anthropic | 2026-10-22 |
| S18 | [MCP Specification 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25) | MCP project | 2025-11-25 |
| S19 | [Introducing advanced tool use](https://www.anthropic.com/engineering/advanced-tool-use) | Anthropic Engineering | 2025-11-24 |
| S20 | [Code execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp) | Anthropic Engineering | 2025-11-04 |
| S21 | [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents) | Anthropic Engineering | 2025-09-29 |
| S22 | [AGENTS.md outperforms skills in our agent evals](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals) | Vercel, Jude Gao | 2026-01-27 |
| S23 | [Evaluating AGENTS.md](https://arxiv.org/html/2602.11988) | Gloaguen et al., ETH Zurich + LogicStar.ai | v1 2026-02-12 (read v3) |
| S24 | [On the Impact of AGENTS.md Files on the Efficiency of AI Coding Agents](https://arxiv.org/abs/2601.20404) | Lulla et al. | 2026-01-28 |
| S25 | [How Many Instructions Can LLMs Follow at Once? (IFScale)](https://arxiv.org/html/2507.11538v1) | Jaroslawicz et al. | 2025-07-15 |
| S26 | [Context Rot](https://www.trychroma.com/research/context-rot) | Chroma: Hong, Troynikov, Huber | 2025-07-14 |
| S27 | [SkillsBench](https://arxiv.org/abs/2602.12670) | Li et al. | v1 2026-02-13; v4 2026-06-14 |
| S28 | [Measuring Claude Code skill activation with sandboxed evals](https://scottspence.com/posts/measuring-claude-code-skill-activation-with-sandboxed-evals) | Scott Spence | 2026-02-08 |
| S29 | [A session-start hook raised Claude Code skill firing from 15% to 62%](https://towardsdatascience.com/a-session-start-hook-raised-claude-code-skill-firing-from-15-to-62/) | Towards Data Science | unknown (HTTP 403) |
| S30 | [Writing a good CLAUDE.md](https://www.humanlayer.dev/blog/writing-a-good-claude-md) | HumanLayer, Kyle | 2025-11-25 |
| S31 | [The /llms.txt file](https://llmstxt.org/) | Jeremy Howard | 2024-09-03 (older); modified 2026-08-10 |
| S32 | [We analyzed 137K sites: 97% of llms.txt files never get read](https://ahrefs.com/blog/llmstxt-study/) | Ahrefs, Louise Linehan | 2026-06-15 |
| S33 | [Otterly / Limy log studies, via PPC Land summary](https://ppc.land/llms-txt-adoption-rises-8-8x-but-97-of-files-get-zero-ai-requests/) | various | 2026 |
| S34 | [Agent Skills in the Wild](https://arxiv.org/abs/2601.10338) | Liu et al. | 2026-01-15 |
| S36 | [Copilot code review: new configurations and controls](https://github.blog/changelog/2026-06-12-copilot-code-review-new-configurations-and-controls/) | GitHub changelog | 2026-06-12 |

## Q1. Agent Skills: spec, disclosure levels, limits, budgets, trigger reliability

### Takeaway
Skills cost little until used (about 100 tokens of metadata each). But agents see only a capped listing. Without help, models fire a skill about half the time.

### Cited Findings
- Spec rules. `name`: 1–64 chars, lowercase letters, digits and hyphens, must match the folder name. `description`: 1–1,024 chars, saying what the skill does and when to use it. Optional: `license`, `compatibility` (≤500 chars), `metadata`, and an experimental `allowed-tools`. [VERIFIED|SHIPPED] ([S1](https://agentskills.io/specification))
- Three loading levels. Metadata (~100 tokens per skill) loads at startup. The body (<5,000 tokens recommended) loads on activation. Other files load only when needed. Keep `SKILL.md` under 500 lines and file references one level deep. Validate with `skills-ref validate`. [VERIFIED|SHIPPED] ([S1](https://agentskills.io/specification))
- Anthropic's advice: third-person descriptions; a table of contents in reference files over 100 lines (nested references may be read only partly); at least 3 evals before writing; test on Haiku, Sonnet and Opus. [VERIFIED|OPINION, vendor] ([S9](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices))
- Claude Code listing: each description is cut at 1,536 chars. The whole listing gets 1% of the context window (`skillListingBudgetFraction` or `SLASH_COMMAND_TOOL_CHAR_BUDGET`). Every name is always listed; descriptions of the least-invoked skills drop first. [VERIFIED|SHIPPED] ([S3](https://code.claude.com/docs/en/skills))
- Claude Code compaction: only the first 5,000 tokens of each invoked skill come back, 25,000 tokens in total. Rules that must hold every time belong in a hook. [VERIFIED|SHIPPED] ([S3](https://code.claude.com/docs/en/skills))
- Claude Code triggers other than the description: `paths` globs, `/skill-name`, and nested `.claude/skills/` folders that load when a file there is touched. `claude plugin eval` measures how often a skill triggers. [VERIFIED|SHIPPED] ([S3](https://code.claude.com/docs/en/skills))
- Codex: reads `.agents/skills` from the working directory up to the repo root, plus user and admin folders. The list gets at most 2% of the window (8,000 chars if the window is unknown). Codex "shortens skill descriptions first" and may "omit some skills". The budget can be raised to 10,000 tokens. [VERIFIED|SHIPPED] ([S12](https://learn.chatgpt.com/docs/build-skills))
- Cursor reads `.agents/skills`, `.cursor/skills`, `.claude/skills` and `.codex/skills`, and supports `paths` ([S13](https://cursor.com/docs/context/skills)). Copilot reads `.github/skills`, `.claude/skills` and `.agents/skills` ([S14](https://docs.github.com/en/copilot/concepts/agents/about-agent-skills)). Gemini CLI reads `.gemini/skills` or `.agents/skills` and asks the user before activating a skill ([S15](https://geminicli.com/docs/cli/skills/)). [VERIFIED|SHIPPED]
- Trigger reliability:
  - Vercel: the skill was never invoked in 56% of cases. An explicit instruction raised triggering to "95%+". [VERIFIED|OPINION] ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals))
  - Spence (Sonnet 4.5, 4 skills, 22 prompts, two runs): no hook 55% and 50%; forced-eval hook 100% and 100%. On 24 harder prompts: forced-eval 75% with 0 false positives; llm-eval 67% with 4 false positives. Prompts with API keywords fired; conceptual phrasing did not. [VERIFIED|OPINION] ([S28](https://scottspence.com/posts/measuring-claude-code-skill-activation-with-sandboxed-evals))
  - Towards Data Science (26 public skills, Claude Code 2.1.316): a session-start hook raised the firing rate from 15% to 62%. Near-miss prompts fired 0 of 972 times. [REPORTED|OPINION] ([S29](https://towardsdatascience.com/a-session-start-hook-raised-claude-code-skill-firing-from-15-to-62/))
- Security: in 31,132 marketplace skills, skills that bundle scripts were 2.12× more likely to have a vulnerability. [REPORTED|RESEARCH] ([S34](https://arxiv.org/abs/2601.10338))

### Inferences
- A router skill (option C) stacks two model decisions. If each fires 50–60% of the time unaided, end-to-end success could be about 25–35%. This assumes the two are independent; nobody has measured it.
- Budget math, assuming ~4 chars per token and ~250-char descriptions:
  - Claude Code at a 200K window: ~2K tokens ≈ 8K chars ≈ 30 full descriptions.
  - Codex at a 200K window: ~4K tokens ≈ 16K chars ≈ 60 full descriptions.
  - At 50 skills, Claude Code drops descriptions. At 200, both tools fall back to names only.
- Four of five tools share `.agents/skills/`. Claude Code needs a `.claude/skills/` symlink or a plugin.

### Gaps
- The docs do not say whether Claude Code's 1% is counted in tokens or chars. They do not say whether Claude Code reads `.agents/skills/`.

## Q2. AGENTS.md and CLAUDE.md

### Takeaway
Both are Markdown files the agent loads every session. The tools agree on the pattern: a short root file plus nested or path-scoped files. But nesting works differently in each tool.

### Cited Findings
- AGENTS.md is plain Markdown with no required fields. The spec says: "The closest AGENTS.md to the edited file wins; explicit user chat prompts override everything." The site says "over 60k open-source projects" use it (undated). OpenAI's main repo had 88 AGENTS.md files. About 23 tools are listed. [VERIFIED|SHIPPED] ([S10](https://agents.md/))
- Governance: the Linux Foundation formed the AAIF on 2025-12-09 to host MCP (from Anthropic), goose (from Block) and AGENTS.md (from OpenAI). Its figures: "more than 60,000" projects use AGENTS.md; there are "more than 10,000 published MCP servers". [VERIFIED|SHIPPED] ([S16](https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation))
- Codex: reads a global file first (`AGENTS.override.md`, else `AGENTS.md`). Then it reads one file per directory, from the project root down to the working directory. Files are joined in order, so later files override earlier ones. `project_doc_max_bytes` defaults to 32 KiB; Codex stops adding files once the total reaches it. `project_doc_fallback_filenames` adds other file names. [VERIFIED|SHIPPED] ([S11](https://learn.chatgpt.com/docs/agent-configuration/agents-md))
- Claude Code [VERIFIED|SHIPPED] ([S4](https://code.claude.com/docs/en/memory)):
  - Files above the working directory load at launch and are joined. Subdirectory files load when a file there is opened.
  - `@path` imports go four hops deep and "don't reduce its context cost".
  - `.claude/rules/*.md` files with `paths:` load when a matching file is touched. On-demand files are cut at 40,000 chars (200K window).
  - Target: "under 200 lines per CLAUDE.md file. Longer files consume more context and reduce adherence." A warning shows past that.
  - There is "no guarantee of strict compliance". Rules that must run at a fixed point should be hooks.
- Claude Code reads AGENTS.md itself since v2.1.277. By default it does so only when no CLAUDE.md exists; a setting loads both. An `@AGENTS.md` import never loads it twice. [VERIFIED|SHIPPED] ([S4](https://code.claude.com/docs/en/memory))
- GitHub removed its 4,000-char cap on code-review instructions on 2026-06-12. [VERIFIED|SHIPPED] ([S36](https://github.blog/changelog/2026-06-12-copilot-code-review-new-configurations-and-controls/))
- HumanLayer: "< 300 lines is best"; its own root file is under 60 lines; task docs live in `agent_docs/`; "Never send an LLM to do a linter's job." [VERIFIED|OPINION] ([S30](https://www.humanlayer.dev/blog/writing-a-good-claude-md))

### Inferences
- echo_image's 82 KB CLAUDE.md is about 20K tokens at ~4 chars per token. That is about 10% of a 200K window every session, before any work starts.
- A per-system AGENTS.md in a subfolder is not portable. Claude Code loads it when a file there is opened. Codex loads only root→working directory, so a session started at the repo root never sees it.

### Gaps
- When a single file is over 32 KiB, the Codex docs do not say whether it is truncated or skipped.

## Q3. llms.txt

### Takeaway
llms.txt is a convention for docs websites. AI crawlers almost never request it. It has no role inside a game repo.

### Cited Findings
- Format: an H1 is the only required part. Then a blockquote summary, H2 link lists, and an "Optional" section agents can skip. Pages should also exist as `.md` copies. Its stated purpose: "mainly useful for inference rather than training". [VERIFIED|SHIPPED] ([S31](https://llmstxt.org/))
- Ahrefs (137,210 domains, May 2026): 28% publish one (an upper bound); 97% of those got zero requests; AI bots made 19.5% of requests. Google's John Mueller called it a "temporary crutch, perhaps to save some tokens" for AI coding tools. [VERIFIED|RESEARCH, vendor] ([S32](https://ahrefs.com/blog/llmstxt-study/))
- Otterly: 84 of more than 62,100 AI-bot visits hit llms.txt over 90 days. Limy: 408 of more than 500M. [REPORTED] ([S33](https://ppc.land/llms-txt-adoption-rises-8-8x-but-97-of-files-get-zero-ai-requests/))
- What I saw while fetching: Claude Code docs, agentskills.io and modelcontextprotocol.io all start their Markdown pages with "Fetch the complete documentation index at …/llms.txt". [VERIFIED|SHIPPED, seen 2026-11-21]

### Inferences
- The pattern that works is "a short index, then fetch one page". That matches Vercel's AGENTS.md index. Inside a repo, a catalog file does the same job without a network fetch.

### Gaps
- No provider says it reads llms.txt automatically.

## Q4. MCP

### Takeaway
MCP fits live actions and state, such as inspecting a running world. It is the wrong tool for static rules. Tool definitions cost a lot of context, but Claude Code now defers most of them.

### Cited Findings
- The spec defines three server features:
  - Resources: "context and data".
  - Prompts: "templated messages and workflows for users".
  - Tools: "functions for the AI model to execute".

  Tool descriptions "should be considered untrusted, unless obtained from a trusted server". [VERIFIED|SHIPPED] ([S18](https://modelcontextprotocol.io/specification/2025-11-25))
- Cost: GitHub's 35 tools take ~26K tokens; 58 tools across 5 servers ~55K. Tool Search cut tokens ~85% and raised MCP eval accuracy from 49% to 74% (Opus 4). [VERIFIED|SHIPPED-beta, vendor numbers] ([S19](https://www.anthropic.com/engineering/advanced-tool-use))
- Code execution with MCP: tools are files in a tree, read on demand. Example: 150,000→2,000 tokens (98.7%). It needs a sandbox. [VERIFIED|OPINION, vendor] ([S20](https://www.anthropic.com/engineering/code-execution-with-mcp))
- Claude Code [VERIFIED|SHIPPED] ([S5](https://code.claude.com/docs/en/mcp)):
  - Tool search is on by default. Only tool names and server instructions load at start. There is "no fixed per-server tool cap".
  - `ENABLE_TOOL_SEARCH=auto` switches at 10% of the context window.
  - Upfront descriptions are cut at 4,096 chars; deferred ones at 16,384.
  - Output warns at 10,000 tokens. The default cap is 25,000 (`MAX_MCP_OUTPUT_TOKENS`).
  - Resources are referenced with `@`. Prompts become `/server:prompt` commands.

### Inferences
- MCP prompts and resources could carry system docs. But the user has to invoke them; agents do not discover them on their own the way they do skills. They also need a running server. Static rules are cheaper as files.

### Gaps
- I did not check Cursor's or Copilot's tool-count limits in this pass.

## Q5. Claude Code plugins and marketplaces

### Takeaway
Plugins are the only shipped format here with versions, dependencies, validation and team rollout. But they work only in Claude Code, and each plugin is sealed to its own folder.

### Cited Findings
- A plugin can bundle skills, agents, hooks, MCP servers, LSP servers, `bin/` executables and evals. A plugin-root CLAUDE.md is not loaded; instructions go in a skill. Every path must stay inside the plugin folder. [VERIFIED|SHIPPED] ([S6](https://code.claude.com/docs/en/plugins-reference), [S7](https://code.claude.com/docs/en/plugins/loading))
- Versioning: `version` "keeps users on that version until you change it"; without it, the 12-char commit SHA is the version. `dependencies` take version constraints. `claude plugin validate --strict` fails CI on warnings. [VERIFIED|SHIPPED] ([S6](https://code.claude.com/docs/en/plugins-reference), [S7](https://code.claude.com/docs/en/plugins/loading))
- Updates: auto-update runs after a random delay of up to 10 minutes. It is on by default for Anthropic's official marketplaces and those added from claude.ai, and off for all others. Old version folders are deleted 14 days later. [VERIFIED|SHIPPED] ([S7](https://code.claude.com/docs/en/plugins/loading))
- Distribution [VERIFIED|SHIPPED] ([S7](https://code.claude.com/docs/en/plugins/loading), [S8](https://code.claude.com/docs/en/plugin-marketplaces)):
  - A marketplace is a repo with `.claude-plugin/marketplace.json`.
  - Plugin sources: relative path, `github`, `git-subdir` (for monorepos), `url`, `archive`, `npm`, `command`. Git sources can be pinned by `ref` or `sha`.
  - Teams share plugins with `enabledPlugins` and `extraKnownMarketplaces` in `.claude/settings.json`, after the trust dialog.
  - Cloud sessions skip marketplaces listed by the repo.
- Gemini CLI also loads skills bundled inside its own extensions. [VERIFIED|SHIPPED] ([S15](https://geminicli.com/docs/cli/skills/))

### Inferences
- A plugin cannot hold code that lives in the game repo. It can ship skills, hooks and checks; the system code itself has to be copied in or imported as a Go module.
- Third-party auto-update is off by default, so games stay pinned unless someone opts in.

### Gaps
- I found no public data on how often teams actually take plugin updates.

## Q6. Evidence on context cost and quality

### Takeaway
All the studies point the same way. More context costs tokens and adherence. The gains come from small, specific, relevant content. Every study has a narrow setup.

### Cited Findings
- **Vercel** (2026-01-27): Next.js 16 APIs missing from training data, graded by build, lint and test. Baseline 53%, skill 53%, skill plus instruction 79%, AGENTS.md index 100%. Compressing the index from 40 KB to 8 KB lost nothing. Weak points: model not named, suite not published, one framework, suite built by the author. [VERIFIED|OPINION] ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals))
- **ETH Zurich + LogicStar.ai** (arXiv v3): four agent/model pairs; SWE-bench Lite (300 tasks) plus 138 tasks from 12 repos with real context files. LLM-written files: −0.5 and −2 pp, +20% and +23% cost. Developer-written: +2.4 pp (p=0.21), up to +19% cost. Agents obey the files: `uv` was used 1.6 times per task when mentioned, under 0.01 when not. Weak points: Python only, bug-fix tasks. [VERIFIED|RESEARCH] ([S23](https://arxiv.org/html/2602.11988))
- **Lulla et al.**: 10 repos, 124 PRs. AGENTS.md went with −28.64% median runtime and −16.58% output tokens. [REPORTED|RESEARCH] ([S24](https://arxiv.org/abs/2601.20404))
- **IFScale** (2025, older models): 500 keyword-inclusion instructions, 20 models; best 68% at 500. claude-opus-4: 100 / 94.6 / 67.9 / 44.6% at 50 / 100 / 250 / 500 instructions. Late instructions are dropped most around 150–200. Weak points: toy task, pre-2026 models. [VERIFIED|RESEARCH] ([S25](https://arxiv.org/html/2507.11538v1))
- **Chroma Context Rot** (2025-07-14): 18 models; performance fell as input grew "in every experiment", more so with distractors. Weak points: synthetic tasks, two topics. [VERIFIED|RESEARCH] ([S26](https://www.trychroma.com/research/context-rot))
- **SkillsBench** (v4): 87 tasks, 18 setups; curated skills 33.9%→50.5%; ≤3-module skills beat exhaustive bundles. [VERIFIED (abstract only)|RESEARCH] ([S27](https://arxiv.org/abs/2602.12670)). v1: skills the model wrote for itself gave no gain. [REPORTED]
- **Anthropic** (2025-09-29): aim for "the smallest set of high-signal tokens"; Claude Code loads CLAUDE.md up front and fetches files with glob and grep when needed. [VERIFIED|OPINION, vendor] ([S21](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents))

### Inferences
- Taken together: the always-loaded file should be a short index plus the rules that hold everywhere. Each system's detail should load on demand. Rules that tests can check should be checked, not repeated in prose.

### Gaps
- No study tests game-engine or Go ECS rule sets. No study measures a 900+ line rules file directly.

## Q7. Discovery

### Takeaway
Agents pick a skill by matching the request against skill descriptions. That fails on vague wording, cut-down listings and long sessions. Fixed triggers are more reliable: file paths, hooks, explicit invocation, and an index that is always loaded.

### Cited Findings
- A good description says what the skill does and when to use it. It uses the words users actually say and puts the key use case first, in case it gets cut. [VERIFIED|SHIPPED] ([S3](https://code.claude.com/docs/en/skills), [S9](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices))
- Documented failure modes [VERIFIED|SHIPPED unless marked]:
  - Bad YAML leaves the skill with empty metadata, so nothing matches it ([S3](https://code.claude.com/docs/en/skills)).
  - A skill is followed once, then forgotten or cut after compaction ([S3](https://code.claude.com/docs/en/skills)).
  - Conflicting rules make the agent pick one at random ([S4](https://code.claude.com/docs/en/memory)).
  - "You MUST invoke the skill" made the agent anchor on the docs and miss a config change ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals)).
  - Conceptual wording failed where API keywords fired ([S28](https://scottspence.com/posts/measuring-claude-code-skill-activation-with-sandboxed-evals)).
  - Near-miss prompts fired 0 of 972 times [REPORTED] ([S29](https://towardsdatascience.com/a-session-start-hook-raised-claude-code-skill-firing-from-15-to-62/)).
  - Codex drops skills that do not fit its budget ([S12](https://learn.chatgpt.com/docs/build-skills)).
- Shipped fixes: `paths` triggers (Claude Code, Cursor); explicit `/skill` or `$skill`; Codex `allow_implicit_invocation: false`; hooks; trigger evals; MCP server instructions that guide tool search. [VERIFIED|SHIPPED] ([S3](https://code.claude.com/docs/en/skills), [S5](https://code.claude.com/docs/en/mcp), [S12](https://learn.chatgpt.com/docs/build-skills), [S13](https://cursor.com/docs/context/skills))

### Inferences
- Once a game has pulled in a system, the file path is the best trigger: editing `pkg/queue/…` loads the queue rules. For choosing which systems to pull in, an always-loaded index beats a router skill.

## Evidence for and against A, B and C

| Option | For | Against | Breaks at (inference) |
|---|---|---|---|
| A. One template + one skill | Context that is always there beat retrieval ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals)). A working template is a set of canonical examples ([S21](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)) | Every system's rules load every session; 966 lines is 4.8× the target ([S4](https://code.claude.com/docs/en/memory)). Adherence falls past ~100–150 instructions ([S25](https://arxiv.org/html/2507.11538v1)). Files add 19–23% cost for ≤2.4 pp ([S23](https://arxiv.org/html/2602.11988)). Exhaustive bundles lose to focused skills ([S27](https://arxiv.org/abs/2602.12670)). The one skill must still trigger | ~10 systems, if the rules stay in one file |
| B. Catalog + one pointer skill | One listing entry, so no budget problem ([S3](https://code.claude.com/docs/en/skills)). An 8 KB index scored 100% when always loaded ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals)) | A pointer *skill* misses 45–56% of the time ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals), [S28](https://scottspence.com/posts/measuring-claude-code-skill-activation-with-sandboxed-evals)). The index grows with the catalog. Rules for pulled-in systems still need a way to load later | ~100–200 systems, unless the index is split into tiers |
| C. One package per system + skill + router | Focused skills win ([S27](https://arxiv.org/abs/2602.12670)). Per-plugin versions and dependencies ([S6](https://code.claude.com/docs/en/plugins-reference)). Portable via `.agents/skills` ([S12](https://learn.chatgpt.com/docs/build-skills)–[S15](https://geminicli.com/docs/cli/skills/)). Trigger rate measurable per skill ([S3](https://code.claude.com/docs/en/skills)) | Listing budgets drop descriptions at roughly 30–60 skills (inference from [S3](https://code.claude.com/docs/en/skills), [S12](https://learn.chatgpt.com/docs/build-skills)). The router adds a second model decision. Compaction keeps only 25K tokens of skills. Plugins work only in Claude Code and are sealed to their own folder ([S7](https://code.claude.com/docs/en/plugins/loading)). Third-party auto-update is off | ~50 systems (descriptions cut); 200 (names only) |

## New options or hybrids

1. **Always-loaded index + detail next to the code.** A small catalog index (aim for ≤8 KB) sits in AGENTS.md. Each system's detail lives in its own folder and loads on demand. This copies Vercel's winning setup and Claude Code's own hybrid. ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals), [S21](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)). The world-engine repo already does this: AGENTS.md links straight to `.agents/skills/*/SKILL.md`.
2. **Rules triggered by file paths.** Each system's rules ship as a `.claude/rules/<system>.md` with `paths:`, or as a skill with `paths:`. They load when the agent touches that code, with no description match needed ([S3](https://code.claude.com/docs/en/skills), [S4](https://code.claude.com/docs/en/memory)). Keep each file under 40,000 chars. In Codex, place nested AGENTS.md files with care ([S11](https://learn.chatgpt.com/docs/agent-configuration/agents-md)).
3. **Checks over prose.** Rules already pinned by tests or mutation checks become hooks or CI steps, not CLAUDE.md lines ([S3](https://code.claude.com/docs/en/skills), [S4](https://code.claude.com/docs/en/memory), [S30](https://www.humanlayer.dev/blog/writing-a-good-claude-md)).
4. **One plugin or skill per family, not per system.** Each holds at most 3 modules, which keeps the listing within budget and matches SkillsBench's sweet spot. (Inference)
5. **MCP only for live engine state and actions**, with tool search on.

## Hype to discount

- Agent Skills: "1 million skills published, nearly 280 million installs" and "70+ platforms". No method is given, and support depth varies by tool ([S17](https://www.anthropic.com/news/donating-agent-skills-to-the-agentic-ai-foundation)).
- AGENTS.md "60k+ projects" is a usage count, not evidence that it helps ([S10](https://agents.md/), [S23](https://arxiv.org/html/2602.11988)).
- Vercel's 100% comes from a self-built, unpublished suite on one framework ([S22](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals)).
- Anthropic's 49→74% and 98.7% are internal evals and an example, not public benchmarks ([S19](https://www.anthropic.com/engineering/advanced-tool-use), [S20](https://www.anthropic.com/engineering/code-execution-with-mcp)).
- Spence's 100% activation rests on 22 prompts and 4 Svelte skills ([S28](https://scottspence.com/posts/measuring-claude-code-skill-activation-with-sandboxed-evals)).
- llms.txt adoption is growing while almost nobody reads the files ([S32](https://ahrefs.com/blog/llmstxt-study/)). "10,000+ MCP servers" counts servers, not quality ([S16](https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation)).

## Gaps

- I could not read the Towards Data Science study (HTTP 403). The 15%→62% and 0/972 figures are search snippets only.
- I did not open the Lulla et al. efficiency study (arXiv 2601.20404).
- From SkillsBench I read only the v4 abstract. The self-generated-skills result is REPORTED.
- The unit of Claude Code's 1% listing budget is not stated. Whether Claude Code reads `.agents/skills/` is not verified.
- Codex's handling of a single AGENTS.md over 32 KiB is unclear.
- I did not check tool-count limits in Cursor and Copilot, or Gemini CLI's AGENTS.md setting.
- Tool churn: the Gemini CLI page says Antigravity CLI replaced it for unpaid users on 2026-06-18. That is a page banner, not checked further.
- No source measures agent accuracy at 50 or 200 skills; another researcher covers that.
