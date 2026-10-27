// Types for wasm_exec.js, the Go toolchain's glue for js/wasm programs. scripts/predictor.mjs copies it
// here from the toolchain that built the wasm, so the two match.

declare global {
  /** Runs one Go program built for js/wasm. */
  class Go {
    readonly importObject: WebAssembly.Imports
    /** Runs the program until it blocks. Settles when it exits. */
    run(instance: WebAssembly.Instance): Promise<void>
  }
}

export {}
