
import path from 'path';

export default {
  // Node.js global to browser globalThis
  define: {
    global: 'globalThis',
},
  build: {
    // disabled because https://github.com/matrix-org/matrix-rust-sdk-crypto-wasm/issues/51
    minify: false,

    // iife required to avoid unminifed variables clashing with global variables
    // e.g calling `var location = {};` at the top-level will immediately redirect
    // the browser to /[object%20Object]
    rollupOptions: {
      input: path.resolve(__dirname, './index.html'),
      output: {
        format: 'iife',
        dir: path.resolve(__dirname, './dist'),
      },
    },
  }

}
