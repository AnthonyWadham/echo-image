# Non-game packaging and update models for AI agents (2025-2026)

Tags: VERIFIED = I read the primary source. REPORTED = search snippet, third-party write-up, or vendor claim I could not check. Plus SHIPPED / ANNOUNCED / OPINION / RESEARCH. "Older" = before 2025. All pages accessed 2026-11-21.

## Key findings

Details and dates are in sections 1-8.

1. **Three vendors put agent docs inside the dependency.** Next.js 16.3, Laravel Boost and Convex ship docs matched to the installed version. A CLI writes a managed AGENTS.md block and refreshes it on upgrade. VERIFIED, SHIPPED. [Next.js guide](https://nextjs.org/docs/app/guides/ai-agents)
2. **Laravel packages ship their own guidelines and skills.** Boost installs them by installed package and version. VERIFIED, SHIPPED. [Boost docs](https://laravel.com/docs/13.x/boost)
3. **Always-loaded docs beat skills in Vercel's eval.** Baseline 53%, skill 53%, skill plus explicit instruction 79%, 8KB AGENTS.md index 100%. It is Vercel's own suite. VERIFIED, RESEARCH. [Vercel blog](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals)
4. **shadcn, the copy-in leader, moves shared code into packages.** `cn` became a package, and shared CSS moved into the `shadcn` package with `eject` as an opt-out. It also added `--diff` to check for upstream updates. VERIFIED, SHIPPED. [cn](https://ui.shadcn.com/docs/changelog/2026-09-cn)
5. **The shadcn registry is a general copy-in catalog.** It carries libs, hooks and any file. It supports item-to-item dependencies and per-item docs, and has an MCP server. VERIFIED, SHIPPED. [registry-item.json](https://ui.shadcn.com/docs/registry/registry-item-json)
6. **Mature ecosystems ship migrations from the package.** Examples: `ng update`, `nx migrate` (with agent-applied prompt migrations), `next upgrade --agent`, and Go's `//go:fix inline`. VERIFIED, SHIPPED. [Nx docs](https://nx.dev/docs/features/automate-updating-dependencies)
7. **Go 1.26 lets a package author rewrite their callers.** The author uses `//go:fix inline`, and `go fix` applies it. VERIFIED, SHIPPED. [Go blog: inliner](https://go.dev/blog/inliner)
   - Inference: a copied system never gets these fixes.
8. **Template-update tools work only for thin templates.** Copier and cruft do text merges with manual conflict review. VERIFIED, SHIPPED. [Copier](https://copier.readthedocs.io/en/stable/updating/)
9. **Laravel copies the starter kit but keeps auth logic in a dependency (Fortify).** In short: copy the shell, depend on the core. VERIFIED, SHIPPED. [Starter kits](https://laravel.com/docs/13.x/starter-kits)
10. **No source tests the owner's claim directly.** Inference from indirect evidence: solid examples work for a first build, but they fail on updates (section 8).

## 1. shadcn: registry, non-UI use, updates to copied code

### Takeaway
shadcn is a shipped catalog of copy-in items that agents can browse. It can now carry any kind of file. Updating copied code is still done by diffing and merging by hand, and shadcn has moved shared code into npm packages.

### Cited Findings
- **Item types and fields.** VERIFIED, SHIPPED. Source: [registry-item.json](https://ui.shadcn.com/docs/registry/registry-item-json), shadcn, undated.
  - Types: `registry:base` (a whole design system), `block`, `component`, `font`, `lib`, `hook`, `ui`, `page`, `file`, `style`, `theme`, and `registry:item` (universal).
  - `files[].target` writes to any project path (`~` = project root).
  - `dependencies` lists npm packages (`name@version`).
  - `registryDependencies` can point to names, `@acme/x`, a GitHub ref such as `acme/ui/button#v1.2.0`, URLs or local paths.
  - `docs` is a message shown when the item is installed.
  - The docs list no field for tests or checks.
- **Universal items (Jul 2025):** "no framework, no components.json, no tailwind, no react required". They can carry "code, config, rules, docs, anything to any code project". VERIFIED, SHIPPED. Source: [Universal Registry Items](https://ui.shadcn.com/docs/changelog/2025-07-universal-registry), Jul 2025.
- **CLI 3.0 (Aug 2025).** It added namespaced `@registry/name` items and private registries (basic auth, bearer token, API key, custom headers). It added `view`, `search` and `list` commands and an MCP server ("Works with all registries. Zero config"). VERIFIED, SHIPPED. Source: [CLI 3.0 and MCP](https://ui.shadcn.com/docs/changelog/2025-08-cli-3-mcp), Aug 2025.
- **MCP server.** Agents can browse, search and install items in plain language. Registries are set in `components.json`. It needs write access and network access, and Codex must be set up by hand. VERIFIED, SHIPPED. Source: [MCP docs](https://ui.shadcn.com/docs/mcp), undated.
- **CLI v4 (Mar 2026).** VERIFIED, SHIPPED. Source: [CLI v4](https://ui.shadcn.com/docs/changelog/2026-03-cli-v4), Mar 2026.
  - `--diff` checks the registry for updates to installed components, so you can merge them with your local changes.
  - It also added `--dry-run`, `--view`, a `shadcn/skills` agent skill, `init --template` for full starter templates, and presets that reconfigure an existing app.
- **Features for large registries.** VERIFIED, SHIPPED. Sources: [Registry include](https://ui.shadcn.com/docs/changelog/2026-05-registry-include), [changelog index](https://ui.shadcn.com/docs/changelog).
  - May 2026: registry `include` splits one large registry across many `registry.json` files.
  - May 2026: `shadcn registry validate` checks the schema, duplicate names and paths.
  - Aug 2026: private GitHub registries. "If you can read the repository, the CLI can install from it."
- **Copied code survives upstream changes.** VERIFIED, SHIPPED. Sources: [changelog](https://ui.shadcn.com/docs/changelog), [cn](https://ui.shadcn.com/docs/changelog/2026-09-cn).
  - After the Oct 2026 AI Elements change, copies "keep working".
  - For `cn`: "Nothing breaks… `lib/utils.ts` keeps working". `shadcn migrate cn` rewrites the imports.
- **The Tailwind v4 refresh overwrote local edits.** It used `add --all --overwrite`. REPORTED, SHIPPED. Source: [Tailwind v4 docs](https://ui.shadcn.com/docs/tailwind-v4), via a search snippet.
- **Supabase uses the registry for logic-heavy blocks.** Its library is "100% shadcn/ui compatible". It ships blocks for password auth, realtime chat, shared cursors and presence avatars. The files live in the user's project. REPORTED, SHIPPED. Sources: [Supabase UI Library blog](https://supabase.com/blog/supabase-ui-library), [Supabase Library FAQ](https://supabase.com/library/docs/getting-started/faq), 2025.
- **Open in v0** opens public registry items. It "does not support cssVars, css, envVars, namespaced registries, or advanced authentication methods". VERIFIED, SHIPPED. Source: [Open in v0](https://ui.shadcn.com/docs/registry/open-in-v0), undated.

### Inferences
- Universal items plus `target` could in theory carry Go files. I found no example of that. Dependencies are npm-only, so Go module dependencies would need another path.
- The format has no field for tests or invariants. Tests would travel as ordinary copied files.

### Gaps
- I could not tell whether `--diff` does a 3-way merge or only shows a diff. I also could not confirm what happened to the older `diff` command.

## 2. Vercel / Next.js / v0

### Takeaway
Next.js treats agent docs as part of the dependency:
- Docs matched to the installed version ship inside the npm package.
- A managed AGENTS.md block points the agent to them.
- Skills are used only for multi-step workflows.
- An agent can run the upgrade.

### Cited Findings
- **Vercel eval** by Jude Gao, 2026-01-27. VERIFIED, RESEARCH (vendor). Source: [Vercel blog](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals).
  - Tasks used Next.js 16 APIs "not in current model training data". Runs were scored on build, lint and test.
  - Pass rates: baseline 53%, skill 53%, skill with an explicit instruction 79%, AGENTS.md docs index 100%.
  - The index was cut from ~40KB to ~8KB "with no loss in pass rate".
  - Caveats: the suite is Vercel's own and was rebuilt once. Results were sensitive to wording ("You MUST invoke the skill" made agents miss project context). An unused skill lowered the test pass rate from 63% to 58%. The post does not give the task count or the models.
- **Next.js docs**, updated 2026-10-15. VERIFIED, SHIPPED. Source: [AI agents guide](https://nextjs.org/docs/app/guides/ai-agents).
  - 16.2+: docs ship at `node_modules/next/dist/docs/`. "Upgrading Next.js also updates the bundled documentation."
  - 16.3+: `next dev` writes, and re-adds, a managed AGENTS.md block between markers and keeps the rest of the file. `create-next-app` creates the agent files.
  - 16.1 and earlier: `npx @next/codemod@canary agents-md` downloads the docs to `.next-docs/`.
- **Same page: skills, upgrades and MCP.** VERIFIED, SHIPPED (`agentUpgrade` and `agentFeedback` are experimental).
  - "Framework knowledge comes from the bundled docs, not from Skills."
  - Skills cover workflows such as adopting Cache Components, with user checkpoints and `instant()` tests. They install with `npx skills add vercel/next.js --skill …`.
  - `next upgrade --agent` collects the migration guides for the target version and hands the upgrade to an agent.
  - The MCP server at `/_next/mcp` exposes `get_compilation_issues` and `compile_route`.
- **Next.js evals**, last run 2026-10-30, pass@4. VERIFIED, RESEARCH (vendor). Source: [nextjs.org/evals](https://nextjs.org/evals).
  - Frontier models score 100% with or without the AGENTS.md pointer.
  - Weaker models gain: Claude Sonnet 5 82%→100%, Kimi K2.7 Code 75%→93%, Kimi K2.5 18%→46% (Apr 2026).
  - One model gained nothing: GPT 5.6 Sol, 79%→79%.
  - The page does not give the task count.

### Inferences
- The AGENTS.md pointer matters most for the weaker, cheaper models that vibe-coders may use.
- Shipping docs inside the dependency prevents examples from drifting away from the installed version.

### Gaps
- I found no Vercel mechanism for updating apps made from Vercel templates. Updates come from the `next` package plus codemods or the agent upgrade. This is inferred from absence. I did not check how v0 works inside, beyond Open in v0.

## 3. Laravel Boost

### Takeaway
Yes, this is a working model. Each package ships its own agent docs. Boost composes them based on which packages and versions are installed. The app can override them, and a command refreshes them.

### Cited Findings
- All findings below are VERIFIED, SHIPPED. Source: [Boost docs 13.x](https://laravel.com/docs/13.x/boost), Laravel, undated.
- **Install.** `composer require laravel/boost --dev`, then `php artisan boost:install`. It writes guideline and skill files for each chosen agent and only replaces text inside its `<laravel-boost-guidelines>` block. The docs say generated files may be gitignored because they are "automatically regenerated".
- **Guidelines.** There are guidelines per package and per major version, for example:
  - Laravel: core, 10.x–13.x
  - Livewire: 2.x–4.x
  - Inertia: 1.x–3.x
  - Pest: 3.x–4.x
  - Tailwind: 3.x–4.x

  The table has 20 rows.
- **Skills.** Skills install based on detected packages: "if your project includes livewire/livewire, the livewire-development skill will be installed". 13 skills are listed.
- **Third-party packages** add `resources/boost/guidelines/core.blade.php` and `resources/boost/skills/{skill-name}/SKILL.md` (Agent Skills format). An app overrides one by using the same path under `.ai/guidelines/…` or the same skill name under `.ai/skills/…`.
- **Updates.** `boost:update` refreshes the files to match installed versions and offers files for newly installed packages. It can run as a Composer `post-update-cmd`.
- **Docs API.** It holds "over 17,000 pieces" of information, searched with embeddings and filtered by installed package versions.
- **Loading.**
  - Guidelines are "loaded upfront".
  - Skills load "on-demand… reducing context bloat".
  - Project rules live in `.ai/rules`, with an `index.md` that maps file globs to rule files. Agents read the index before editing.

### Inferences
- In Boost, the package manager acts as the router: installed packages decide which docs land. No router skill is needed.
- Vendors disagree on whether to commit the generated agent files. Boost lets you gitignore them; Next.js tells agents to commit its block.

### Gaps
- I found no published eval numbers for Boost.

## 4. Convex components and Chef

### Takeaway
Convex components are isolated backend modules distributed on npm. Chef is one template on top of a strong platform API. How apps upgrade a component is not documented.

### Cited Findings
- **Components.** They are "mini self-contained Convex backends, and installing them is always safe". Each has its own tables and isolated function execution. They "can't read your app's tables or call your app's functions unless you pass them in explicitly". They install from npm or from a local folder. VERIFIED, SHIPPED. Source: [Components](https://docs.convex.dev/components), undated.
- **Authoring.** VERIFIED, SHIPPED. Source: [Authoring](https://docs.convex.dev/components/authoring), undated.
  - Authors use `defineComponent`; apps install with `app.use()`.
  - A local `convex/components` folder is "the easiest way to get started". Packages expose a `/test` entry and are tested with `convex-test`.
  - The docs warn: "hybrid components introduce a lot of complexity for maintaining and updating the component in backwards-compatible ways."
  - The docs give no guidance on version upgrades or schema migrations.
- **`npx convex ai-files`.** VERIFIED, SHIPPED. Source: [ai-files](https://docs.convex.dev/cli/reference/ai-files), undated.
  - It manages `convex/_generated/ai/guidelines.md`, the Convex section of AGENTS.md/CLAUDE.md, and skills.
  - `status` compares local files with "latest hashes from version.convex.dev".
  - `npx convex dev` prompts when the files are stale.
  - A third-party post says the command arrived in convex 1.34.0. REPORTED.
- **Chef.** VERIFIED, SHIPPED. Source: [get-convex/chef](https://github.com/get-convex/chef), undated.
  - It calls itself "The only AI app builder that knows backend".
  - Its README says: "The 'magic' in Chef is just the fact that it's using Convex's APIs, which are an ideal fit for codegen."
  - `template/` is the "starter template for every Chef project". The system prompt is published with each release.
  - It is Apache-2.0 and a fork of bolt.diy.

### Inferences
- Chef supports option A only in a thin sense: one small template on top of a rich dependency platform. The systems live in the platform, not in the template.
- Built-in isolation, where a component cannot touch app data, is a guardrail that copied examples cannot provide.

### Gaps
- I could not confirm whether Chef installs components, or how apps upgrade component versions.

## 5. Supabase, Angular, Nx

### Takeaway
All three keep the logic as a dependency and add agent tooling on top. Angular and Nx ship migrations owned by each package. Nx now lets an agent apply some of them.

### Cited Findings
- **Supabase Agent Skills**, by Pedro Rodrigues, 2026-04-09. VERIFIED, RESEARCH (vendor). Source: [Supabase blog](https://supabase.com/blog/supabase-agent-skills).
  - The core `SKILL.md` is ~100 lines, at v0.1.0. It installs with `npx skills add supabase/agent-skills` or as a Claude Code plugin.
  - The eval used six scenarios per condition and an LLM judge. Results (baseline / MCP only / MCP plus skill):

    | Model | Baseline | MCP only | MCP + skill |
    |---|---|---|---|
    | Opus 4.6 | 58% | 50% | 67% |
    | Sonnet 4.6 | 46% | 58% | 71% |
    | GPT-5.4 | 71% | 71% | 88% |
    | GPT-5.4 Mini | 42% | 63% | 71% |

  - Supabase calls these "early results with a small sample size".
  - Agents with MCP only never called `search_docs`.
- **Supabase also ships static prompt files** to copy into a project, plus an MCP server it says not to connect to production. REPORTED, SHIPPED. Source: [AI Tools docs](https://supabase.com/docs/guides/ai-tools).
- **Angular CLI MCP** (docs v22.3). VERIFIED, SHIPPED. Source: [Angular MCP](https://angular.dev/ai/mcp), undated.
  - Tools: `get_best_practices` ("version-aligned coding rules"), `search_documentation`, `onpush_zoneless_migration`, `run_target`, `devserver_*`, `ai_tutor`.
  - Flags: `--read-only` and `--local-only`.
- **`ng update`.** It has `--migrate-only`, `--from`/`--to`, `--name` and `--create-commits`. Libraries use it "to adjust their project for a new version of your library that introduces breaking changes". VERIFIED, SHIPPED. Sources: [ng update](https://angular.dev/cli/update), [schematics for libraries](https://angular.dev/tools/cli/schematics-for-libraries), undated.
- **`nx migrate`.** VERIFIED, SHIPPED. Source: [Nx docs](https://nx.dev/docs/features/automate-updating-dependencies), undated.
  - It updates versions and writes `migrations.json`; `--run-migrations` then applies them. Each plugin ships its own migrations.
  - Migration types are generator-based, prompt-based ("can't be expressed deterministically") and hybrid.
  - With an agent (Claude Code, Codex or OpenCode), the agentic flow:
    - runs each generator, then the agent applies the prompt
    - checks lint, build and unit tests
    - commits each migration separately
    - can resume from `.nx/migrate-runs`
  - Nx recommends one major version at a time.

### Inferences
- Nx prompt-based migrations are the closest shipped answer to "how do breaking changes reach AI-written projects". The package ships the instructions, the agent applies them, and tests gate the result.

### Gaps
- Supabase's percentages cannot all come from six scenarios (for example, 58% and 46% are not whole fractions of six). The method is unclear.

## 6. Template-update tools (Copier, cruft)

### Takeaway
They work for thin scaffolds when teams are disciplined. They break down with heavy customization, long gaps between updates, and deleted files.

### Cited Findings
- **Copier update.** VERIFIED, SHIPPED. Source: [Copier: Updating](https://copier.readthedocs.io/en/stable/updating/), undated.
  - It needs `.copier-answers.yml`, git tags on the template, and a project under git.
  - It regenerates the old version, diffs it against the project, applies the new template, then re-applies the diff. Pre- and post-migrations run around this.
  - Conflicts appear inline (the default) or as `.rej` files.
  - Never edit the answers file by hand.
  - Deleted files stay deleted. `recopy` overrides local changes.
  - Aborting a failed update is awkward.
  - Templates should be "as pure and simple as possible".
- **Real use at Diamond Light Source.** Their update guide runs `uvx copier update`, validates with `tox -p` before and after, and uses `git diff --check` to find conflicts. The guide mentions no staleness check in CI. VERIFIED, SHIPPED. Source: [DLS how-to](https://diamondlightsource.github.io/python-copier-template/main/how-to/update-template.html), undated.
- **cruft.** VERIFIED, SHIPPED. Source: [cruft docs](https://cruft.github.io/cruft/), undated.
  - `.cruft.json` records the template commit and the variables.
  - `cruft check` exits 1 when the project is behind, so it can block drift in CI.
  - `cruft diff --exit-code` fails when local files differ from the template.
  - The docs include a weekly GitHub Action that opens update PRs.
  - The docs admit "certain files just aren't good fits for updating".

### Inferences
- These tools merge text without knowing what the code means. That is fine for config and CI files, but poor for gameplay logic that agents have edited.

### Gaps
- I found no numbers on success at the scale of many projects, and did not check cruft's recent release activity.

## 7. Go: module versioning, multi-module repos, go fix

### Takeaway
Go has the strongest built-in tools for updating dependencies of any ecosystem here. All of them are keyed to module versions.

### Cited Findings
- **Module rules.** VERIFIED, SHIPPED (these features are older). Source: [Go Modules Reference](https://go.dev/ref/mod), undated.
  - From v2 on, import paths need a `/v2` suffix, and modules with different suffixes are separate modules.
  - `retract` (Go 1.16): users are not upgraded to a retracted version, builds keep working, and `go list -m -u` / `go get` warn.
  - A module can be marked `// Deprecated:` (checked since Go 1.17).
  - Version selection is MVS.
- **Go 1.26 (Feb 2026).** `go fix` was rewritten on the `go vet` analysis framework. It now has "dozens of fixers" plus an inliner driven by `//go:fix inline`, so authors can "automate their own API migrations". VERIFIED, SHIPPED. Source: [Go 1.26 notes](https://go.dev/doc/go1.26).
- **Go blog, Alan Donovan, 2026-02-17.** VERIFIED; SHIPPED, with the last point ANNOUNCED. Source: [Using go fix](https://go.dev/blog/gofix).
  - Fixes apply only where a file's Go version allows them.
  - Rerun `go fix` after each toolchain upgrade.
  - LLM tools "tended" to write in the style of the code they were trained on.
  - The team is exploring loading modernizers from the source tree.
- **Go blog, Alan Donovan, 2026-03-10.** VERIFIED, SHIPPED. Source: [//go:fix inline](https://go.dev/blog/inliner).
  - It works on functions, type aliases and constants. Preview with `go fix -diff ./...`; gopls shows a diagnostic with a quick fix.
  - It is conservative: it inserts parameter bindings when needed and refuses functions that use `defer`.
  - It prepared more than 18,000 changelists at Google.
- **Go 1.27 (Aug 2026).** It added modernizers `atomictypes`, `embedlit`, `slicesbackward` and `unsafefuncs`. `go test` now runs the `stdversion` vet check, and `go doc pkg@version` works. VERIFIED, SHIPPED. Source: [Go 1.27 notes](https://go.dev/doc/go1.27).
- **Multi-module release tooling.** VERIFIED, SHIPPED. Sources: [multimod](https://github.com/open-telemetry/opentelemetry-go-build-tools/tree/main/multimod), [otel-contrib versions.yaml](https://raw.githubusercontent.com/open-telemetry/opentelemetry-go-contrib/main/versions.yaml).
  - multimod defines module sets in `versions.yaml`. `verify` checks that each module is in exactly one set and warns when a stable module depends on an unstable one. `prerelease` and `tag` cut releases.
  - opentelemetry-go-contrib has 69 modules in 8 sets. For example, stable-v1 (v1.47.0) has 9 modules and experimental-instrumentation (v0.72.0) has 27. 9 example modules are excluded.
- **aws-sdk-go-v2 versions each service as its own module.** Tags look like `service/s3/v1.31.2`, and service changelogs carry "Dependency Update: Updated to the latest SDK module versions". REPORTED, SHIPPED. Sources: [AWS GA blog](https://aws.amazon.com/blogs/developer/aws-sdk-for-go-version-2-general-availability), Jan 2021 (older); search results. VERIFIED from the [README](https://github.com/aws/aws-sdk-go-v2): it needs Go 1.24 or later.
- **`gonew` (2023-07-31)** copies a template module under a new module path. It is still labeled "highly experimental". REPORTED, SHIPPED (older). Sources: [Go blog: gonew](https://go.dev/blog/gonew), [pkg.go.dev](https://pkg.go.dev/golang.org/x/tools/cmd/gonew).

### Inferences
- Copied Go code loses `go get` updates, retract and deprecation warnings, and the system author's `//go:fix inline` migrations.
- OpenTelemetry versions its ~70 modules in 8 sets, not one by one.
- The Go team's own template tool has stayed experimental for 3+ years. That is a weak signal about demand for copy-in in Go.

### Gaps
- I did not check how govulncheck treats copied code, or AWS's internal release tooling.

## 8. Copy-in vs dependency, and "examples are as good as plugins"

### Takeaway
Mature ecosystems use both. They copy what users must own: UI, glue code and scaffolding. They depend on code that holds invariants and needs fixes. The evidence supports examples for building a first version. It does not support them for keeping a project up to date.

### Cited Findings
- **Laravel starter kits.** VERIFIED, SHIPPED. Source: [Starter kits](https://laravel.com/docs/13.x/starter-kits), undated.
  - The code "exists within your application". The FAQ says "there is no need to update the starter kit itself".
  - Auth comes from the Fortify dependency ("routes, controllers, and logic"). Apps customize it through action classes they own in `app/Actions/Fortify`.
- **shadcn moved toward dependencies:** `cn` as a package; shared CSS in the `shadcn` package, with eject stamping the version (`ejected from shadcn@4.8.3`); `--diff`; migrate commands. VERIFIED, SHIPPED (section 1).
- **GitClear says AI raises duplication.** REPORTED, RESEARCH. Sources: [GitClear 2025](https://www.gitclear.com/ai_assistant_code_quality_2025_research), [DevClass](https://www.devclass.com/ai-ml/2025/02/20/ai-is-eroding-code-quality-states-new-in-depth-report/1626250) (2025-02-20), [GitClear 2026](https://gitclear.com/the_ai_code_quality_maintainability_gap).
  - Its 2025 study covered 211M changed lines (2020–2024). Duplicated blocks of 5+ lines rose 8x in 2024.
  - Its 2026 follow-up reports 73 duplicated lines per 1,000 changes.
  - Google's DORA 2024 report disagreed: it said AI raised code quality by 3.4%.
- **Agents fall back to their training data.** They write code in the style they were trained on (Go blog). On APIs released after training, they fail without docs matched to the version (Vercel's 53% baseline). VERIFIED.
- **Skill distribution with `npx skills`.** `skills-lock.json` stores content hashes, and `skills update` reinstalls changed skills. A third-party tool says plain `skills add` "fetches whatever is on main", with no pinning. REPORTED. Sources: [skills lock files (mirror)](https://www.mintlify.com/vercel-labs/skills/advanced/lock-files), [skills-lock](https://github.com/luisalima/skills-lock).

### Inferences
- With copy-in, three things break first:
  1. Shared helpers get duplicated (shadcn's `cn`).
  2. Upstream fixes need a diff and a manual merge.
  3. Each breaking change needs its own migrate command.
- With dependencies, the risks are version skew and breaking changes. Semver, `retract` and shipped migrations reduce them.
- echo_image is one Go module (`./go.mod`, with `pkg/level`, `pkg/levelstate` and `pkg/queue`). The client runs the server's Go code in wasm. If each game copies a system, client and server can diverge across games. A dependency pins both to one version.

## Evidence for and against A, B and C

**A. One full template plus a skill**
- For:
  - Chef uses one `template/` for every project. VERIFIED.
  - Laravel's starter kits and shadcn's `init --template` give users the code to own. VERIFIED.
  - Always-loaded context beat skills (Vercel), so a template that carries a compact index has support.
- Against:
  - Templates have no update path. Laravel: "no need to update the starter kit".
  - Copier and cruft exist because templates drift, and they need text merges and manual conflict review.
  - Chef credits Convex's APIs (a dependency), not its template.
  - Inference: with 50–200 systems, every game would carry and prune systems it does not use.

**B. A catalog plus one pointer skill**
- For: shadcn's registry is exactly this, and it ships. It has namespaced, private and GitHub registries; search, view and list; MCP installs; item-to-item dependencies; per-item `docs`; and `include` and `validate` for large registries. Supabase and v0 use it.
- Against:
  - Updates are a diff plus a hand merge.
  - shadcn itself moved shared code into packages.
  - A skill that is not invoked adds nothing: 53% against a 53% baseline (Vercel).
  - The registry format has no field for tests or checks.
  - Open in v0 cannot use namespaced registries.

**C. Separate packages, each with its own doc and skill, plus a router**
- For:
  - Laravel Boost is this model, shipping, minus the router.
  - Next.js ships docs inside its package.
  - Convex components are isolated packages, with AI files checked for staleness.
  - Angular and Nx packages ship their own migrations.
  - Go multi-module repos exist at scale (69 modules at OpenTelemetry; one module per AWS service).
- Against:
  - The router is another on-demand step. Next.js and Laravel keep the core index always loaded and use skills only for workflows or details.
  - At ~70 modules OpenTelemetry uses version sets and release tooling. AWS bumps get noisy (REPORTED).
  - Convex warns that mixing local and shared code is hard to keep backward compatible.

## New options or hybrids

1. **Dependency plus version-matched docs plus a managed AGENTS.md block.** The package manager decides which docs load; no router skill is needed. A refresh runs on upgrade. Used by Next.js 16.3, Laravel Boost and Convex `ai-files`.
2. **Copy the shell, depend on the core.** Laravel copies the kit but keeps auth in Fortify, with hooks the app owns. shadcn moved `cn` into a package.
3. **Dependency with eject.** `shadcn eject` copies the source and stamps the version, which makes later diffs possible.
4. **Migrations ship with the package.** Examples: `//go:fix inline` for renames; Nx prompt-based migrations that an agent applies and tests gate; `next upgrade --agent`.
5. **Copier only for a thin scaffold.** Keep templates "pure and simple".
6. **A small always-loaded index plus on-demand detail.** Vercel's 8KB index, or Boost's `.ai/rules/index.md` that maps globs to rule files.

## Hype to discount

- **Supabase "100% success rate on security-critical tasks".** This comes from a talk summary (REPORTED). The blog's own numbers are 67–88% on six scenarios, scored by an LLM judge.
- **Vercel's "100% vs 79%".** The suite is Vercel's own, with no task count or models given. Its public evals now show frontier models at 100% even without the pointer.
- **Laravel's "17,000 pieces".** A count, not a quality measure.
- **Chef's "only AI app builder that knows backend".** A tagline.
- **GitClear's 8x.** The vendor sells code analytics, and DORA disagrees.
- **Go's 18,000 changelists.** Google-internal. I found no adoption numbers from the open ecosystem.
- **shadcn MCP "Zero config".** Its own docs list requirements.

## Gaps

- No study directly compares copied examples with a dependency, for either agent success or maintenance.
- No data on how often vibe-coded projects ever update.
- Task counts and models for the Vercel and Next.js evals.
- What shadcn `--diff` does exactly (3-way merge or a plain diff).
- How Convex components are upgraded, and whether Chef uses components.
- How govulncheck treats copied code, and AWS's internal release tooling.
- Official pinning rules for `npx skills`.
- How apps made from Vercel templates get updates.
- Success rates for Copier or cruft across many projects.
