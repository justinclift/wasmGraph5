### Another simple example using Go Wasm with the HTML5 Canvas (2D)

Online demo: https://justinclift.github.io/wasmGraph5/

This renders points of a basic 2D equation, and it's first derivative,
onto the canvas.  It uses an [external library](https://godoc.org/github.com/corywalker/expreduce) for doing the math, which
seems to slow things down to an extreme level.  Previously this demo
loaded in under a second, now it's taking 10-15+ seconds after loading
to start.

Use the wasd, arrow, and numpad keys (including + and -) to rotate the
graph around the origin.  Use the mouse wheel to zoom in and out.

The code for this started from https://github.com/stdiopt/gowasm-experiments,
and has been fairly radically reworked from there. :smile:
