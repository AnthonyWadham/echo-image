import './wasm_exec.js' // defines globalThis.Go

/** Starts a Go program built for js/wasm. It runs until it first blocks, so what it sets up is there on return. */
export async function runGo(wasm: BufferSource): Promise<void> {
  const go = new Go()
  const { instance } = await WebAssembly.instantiate(wasm, go.importObject)
  void go.run(instance) // settles only when the program exits
}
