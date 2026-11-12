# The predictor on every client: browser, Unity, phones and consoles

Researched 2026-11-12. Research only: no code has changed. Code as of echo_image `3f26a8b`.

> **These are findings, not the source of truth.** Code and [`CLAUDE.md`](../../CLAUDE.md) win when
> they disagree with this file. Each claim says how sure it is: **[verified]** was read in the code or
> run here, **[sourced]** has a link in section 9, **[from memory]** was not checked. Re-check a fact
> before you build on it.

## Summary

- **The question.** The predictor is Go, built to wasm, and runs in the browser. A Unity client can
  load Go as a native plugin on PCs and phones, but not on consoles. Is there no way to predict on a
  console?
- **The answer: there is a way.** "Consoles cannot run Go" only means the Go compiler has no console
  setting. The code can still get there in steps: Go to wasm, wasm to C, C to the console.
- **The pick for all games.** Ship the same Go sim to every client as one wasm file. The browser runs
  it as it is. Unity gets the same wasm turned into C. World Engine should own that pipeline, so each
  game does not build its own.
- **Echo is most of the way there.** The pure core, the wasm build, the self-fixing client and the
  bit-for-bit test exist. What is missing is plumbing: an entry point that is not tied to the browser,
  the wasm-to-C step, and a Unity plugin.
- **One real unknown.** Nobody has shown wasm-to-C works well for a Go program of our size. A test of
  3 to 5 days answers it (steps 1 and 2 in section 7).
- **Advice.** Do nothing until a Unity or console client for Echo is really planned. Then do the test
  first and decide from its result.

## 1. In plain words

- The server runs the game. Your screen cannot wait for it, so your device guesses what the server
  will say. That guess is the predictor.
- The best guess comes from running the same rules the server runs.
- Think of the rules as a recipe. It is written in Go. A machine can translate it, step by step, into a
  form any device can read. If each translation is exact, every device bakes the same cake.
- No device ever runs "Go". Go is what people write. A compiler turns it into instructions for one
  kind of machine. The Go compiler has settings for PCs, phones and servers, and none for consoles.
- wasm is a set of instructions for a pretend computer that every device agrees on. The Go compiler
  can write wasm. Another tool turns wasm into C, and every console's own compiler reads C.
- The server always has the last word. When a guess is wrong, the device fixes itself.

## 2. Why consoles are the hard case

- Go has no PlayStation, Xbox or Switch target, so the usual Go plugin does not build there
  **[from memory]**. Go does build plugins for Windows, macOS, Linux, Android and iOS.
- Consoles do not let a program make new machine code while it runs **[from memory]**. Most wasm
  runtimes work that way, so they are out too. iOS has the same rule.
- So a console needs code that is fully compiled before it ships.

## 3. The three ways

| Way | How | Shown to work? |
|---|---|---|
| 1. wasm to C | Turn `predictor.wasm` into plain C with `wasm2c`. Compile it with each platform's own compiler. | The tool is mature: Firefox ships code built this way **[sourced]**. Ebitengine used this route for Switch, then replaced it with way 2 **[sourced]**. |
| 2. Patched Go | Build Go for the console's chip, with its system calls swapped for C calls (Hitsumabushi). | Yes: Ebitengine games shipped on Switch and Xbox **[sourced]**. Nothing found for PS5. |
| 3. Rewrite in C# | Port the predictor by hand and let the server correct it. | Yes, as a method. Rampage works this way. |

Trade-offs:

- **Way 1** keeps one copy of the sim, and one artifact for every client. It is slower than native
  Go, which matters little for a small predictor. Go 1.24 and later can build a wasm library with
  exported functions, which this needs **[sourced]**. Why Ebitengine left this route is not in the
  sources found; they run a whole game through it, and we would run a predictor.
- **Way 2** is the fastest. It needs each console's SDK, and its patches follow each Go version. Its
  docs say it covers only Linux/arm64 and Windows/amd64 so far **[sourced]**.
- **Way 3** means two copies of the sim for each game. Our Box2D is Go; matching it exactly in C# is
  hard, so players would see small corrections.

## 4. Why the results match

- The rules are a long list of maths steps. A translation keeps the same steps in the same order.
- wasm has a strict rulebook: it says what each maths step gives, to the last digit, on every device
  **[from memory]**. So every client that runs the same wasm gets the same numbers.
- The physics comes along. World Engine's Box2D is pure Go, and the predictor already makes its own
  Box2D world **[verified]**. If it were a C library, this would be much harder.
- The one pair that can differ is the server (Go built directly) against the wasm. On arm64, a direct
  Go build may merge a multiply and an add into one step that rounds once. Echo's code is written to
  avoid that, and a test compares the two bit for bit at 20 and 60 Hz **[verified]**.
- A rounding difference is about the 15th digit. A fix that small cannot be seen.
- **The new risk in way 1.** A C compiler may also merge or reorder maths unless told not to. The
  setting differs by compiler **[from memory]**. It must be set once in the build, and proved by a
  test, not by trust: each platform replays the recorded match and compares with the server.
- What the self-fixing is really for: things no device can know yet. Another player stunned you, or a
  message was lost. Every game has these, and they are smoothed over a few frames.

## 5. What it costs the server

- Prediction adds almost nothing: one number in each message, "I have handled your input up to X".
- The guessing runs on each player's own device. More players means more devices doing their own
  work, not a slower server.
- The device predicts only what its player controls. In Echo that is your Body, your Echo and your
  shots **[verified]**.
- Lag compensation (the `history` feature) does cost the server. It is a separate feature. A game can
  have prediction without it.

## 6. The design for all games

| Part | Owner | What |
|---|---|---|
| A pure sim core | each game | Rules that need prediction live in Go packages with no Cardinal import: state and input in, state out. |
| One build, one artifact | World Engine | The CLI builds the core to wasm. The browser runs it. Unity gets the same wasm as C. |
| Always reconcile | the SDK | The client rolls back and replays when the server disagrees. An exact match is a goal that tests measure, not something a game depends on. |

- Every engine with prediction runs the same gameplay code on both sides (Unreal, Source, Unity
  Netcode, Photon) **[from memory]**. None writes the sim twice. Our server is Go and our clients are
  not, so the question is only how the Go reaches the client.
- Not chosen: native Go for each platform (five toolchains, console patches for each Go version, and
  different rounding on different chips), and a C# rewrite (two sims for each game, for ever).
- If an engine were started today with consoles as a goal, a sim language that compiles everywhere
  (C++, Rust, C#) would be the easier choice. With Go, wasm is the bridge.

## 7. What echo_image needs

Done **[verified]**:

| Piece | State |
|---|---|
| Rules in a pure core | Done: 459 lines, no Cardinal import, only Box2D and our own pure packages. |
| Built to wasm | Done: 5.3 MB. |
| The browser runs it | Done. |
| The client fixes itself | Done: it checks each sync, resets and replays. |
| Server and wasm match bit for bit | Done: tested at 20 and 60 Hz, on the Go side and in the client's tests. |

Missing. The sizes are estimates, not measurements:

| # | Work | Size | Risk |
|---|---|---|---|
| 1 | A second entry point that is not tied to the browser. Today's (217 lines) speaks only JavaScript. The new one takes bytes and gives bytes, in the MessagePack the wire already uses. | 1–2 days | Low |
| 2 | Turn that wasm into C and run it on a PC. Replay the recorded match and compare with the server. | 2–3 days | **The unknown.** It shows speed, code size, and whether it matches. |
| 3 | A Unity plugin around the C, for the editor, desktop and phones. | 3–5 days | Medium |
| 4 | Prediction in the Unity client: the job two browser files do today (about 350 lines). | 3–5 days | Medium |
| 5 | The replay test on each platform. | 1 day after step 2 | Low |
| 6 | Consoles. | Not estimated | Needs dev kits and SDK access. |

- Steps 1 and 2 answer the whole question. If the C build matches the server on a PC, the rest is
  normal work. If it does not, we stop having lost under a week.
- The Unity client is stale. It still speaks the old protobuf format and cannot talk to this shard
  **[verified in `CLAUDE.md`]**. That comes before step 4 and is separate work.
- Moving the pipeline into World Engine comes after Echo proves it. Echo is the test case.
- A bonus: if the browser later uses the new entry point too, the copy of `wasm_exec.js` goes away.
  That copy broke the client's tests on 2026-11-12, when the installed Go and the pinned Go differed.
- The wasm should be built once, by the Go that `go.mod` pins, and the same bytes should go to the
  browser and to the wasm-to-C step. Then there is one artifact to hash and to test.

## 8. Open questions

- How large is the C that `wasm2c` writes for a 5.3 MB Go wasm, and how long does it take to build?
- How fast is it? The client replays several ticks after a correction, so a tick must stay cheap.
- Which host calls does the Go wasm library need, and how small can the C that answers them be?
- Does each console's compiler have a setting that keeps maths steps apart?
- May a console or iOS game download a new wasm and run it in an interpreter? If so, the server could
  hand each client the exact predictor it needs. Not researched.
- Does Hitsumabushi cover PS5? Nothing found.

## 9. Sources

- Ebitengine on Nintendo Switch: <https://ebitengine.org/en/blog/nintendo_switch.html>
- Ebitengine, a native Switch binary from Go: <https://ebitengine.org/en/blog/native_compiling_for_nintendo_switch.html>
- Ebitengine 2.3 release notes (Go to C++ by way of wasm): <https://ebitengine.org/en/documents/2.3.html>
- Ebitengine 2.5 release notes (Xbox): <https://ebitengine.org/en/documents/2.5.html>
- Hitsumabushi package docs: <https://beta.pkg.go.dev/github.com/hajimehoshi/hitsumabushi>
- Go blog, wasm libraries and `go:wasmexport`: <https://go.dev/blog/wasmexport>
- `wasm2c` README: <https://cdn.jsdelivr.net/gh/WebAssembly/wabt@1.0.40/wasm2c/README.md>
- Firefox's RLBox build, which uses `wasm2c`: <https://searchfox.org/firefox-main/source/security/rlbox/rlbox.mozbuild>

## Appendix: where things are

| What | Where |
|---|---|
| The pure core | `shards/game/internal/predict/predict.go` |
| The browser entry point | `shards/game/predictor/main.go` |
| The build script | `client/scripts/predictor.mjs` |
| The browser side | `client/src/game/player/predictor.ts`, `prediction.ts` |
| The bit-for-bit test | `shards/game/test/prediction_test.go` (`TestPredictionMatchesServer`), `client/src/game/player/predictor.test.ts` |
| The recorded match's digests | `shards/game/test/testdata/prediction-20hz.txt`, `prediction-60hz.txt` |
| The sim version | `sim.go` |
| The Unity client | `/mnt/z/Echo` |
