//Wasming
// compile: GOOS=js GOARCH=wasm go build -o main.wasm ./main.go
package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	eq "github.com/corywalker/expreduce/expreduce"
	"go.uber.org/atomic"
)

type matrix []float64

type Point struct {
	Label      string
	LabelAlign string
	X          float64
	Y          float64
	Z          float64
}

type Edge []int
type Surface []int

type Object struct {
	C    string // Colour of the object
	P    []Point
	E    []Edge    // List of points to connect by edges
	S    []Surface // List of points to connect in order, to create a surface
	Name string
	Eq   string // Used to store the equation for graph and derivative objects
}

type OperationType int

const (
	ROTATE OperationType = iota
	SCALE
	TRANSLATE
)

type Operation struct {
	op OperationType
	t  int32 // Number of milliseconds the operation should take
	f  int32 // Number of display frames the operation should be broken into
	X  float64
	Y  float64
	Z  float64
}

const (
	sourceURL = "https://github.com/justinclift/wasmGraph5"
)

var (
	// The empty world space
	worldSpace []Object

	// The point objects
	axes = Object{
		C:    "grey",
		Name: "axes",
		P: []Point{
			{X: -0.1, Y: 0.1, Z: 0.0},
			{X: -0.1, Y: 10, Z: 0.0},
			{X: 0.1, Y: 10, Z: 0.0},
			{X: 0.1, Y: 0.1, Z: 0.0},
			{X: 10, Y: 0.1, Z: 0.0},
			{X: 10, Y: -0.1, Z: 0.0},
			{X: 0.1, Y: -0.1, Z: 0.0},
			{X: 0.1, Y: -10, Z: 0.0},
			{X: -0.1, Y: -10, Z: 0.0},
			{X: -0.1, Y: -0.1, Z: 0.0},
			{X: -10, Y: -0.1, Z: 0.0},
			{X: -10, Y: 0.1, Z: 0.0},
			{X: 10, Y: -1.0, Z: 0.0, Label: "X", LabelAlign: "center"},
			{X: -10, Y: -1.0, Z: 0.0, Label: "-X", LabelAlign: "center"},
			{X: 0.0, Y: 10.5, Z: 0.0, Label: "Y", LabelAlign: "center"},
			{X: 0.0, Y: -11, Z: 0.0, Label: "-Y", LabelAlign: "center"},
		},
		E: []Edge{
			{0, 1},
			{1, 2},
			{2, 3},
			{3, 4},
			{4, 5},
			{5, 6},
			{6, 7},
			{7, 8},
			{8, 9},
			{9, 10},
			{10, 11},
			{11, 0},
		},
		S: []Surface{
			{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		},
	}

	// The 4x4 identity matrix
	identityMatrix = matrix{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}

	// Initialise the transform matrix with the identity matrix
	transformMatrix = identityMatrix

	// FIFO queue
	queue        chan Operation
	renderActive *atomic.Bool

	width, height       float64
	graphWidth          float64
	graphHeight         float64
	cCall, kCall, mCall js.Callback
	rCall, wCall        js.Callback
	ctx, doc, canvasEl  js.Value
	eqStr, derivStr     string
	opText              string
	highLightSource     bool
	pointStep           = 0.05
	debug               = false // If true, some debugging info is printed to the javascript console
)

func main() {
	// Initialise canvas
	doc = js.Global().Get("document")
	canvasEl = doc.Call("getElementById", "mycanvas")
	width = doc.Get("body").Get("clientWidth").Float()
	height = doc.Get("body").Get("clientHeight").Float()
	canvasEl.Call("setAttribute", "width", width)
	canvasEl.Call("setAttribute", "height", height)
	canvasEl.Set("tabIndex", 0) // Not sure if this is needed
	ctx = canvasEl.Call("getContext", "2d")

	// Set up the mouse click handler
	cCall = js.NewCallback(clickHandler)
	doc.Call("addEventListener", "mousedown", cCall)
	defer cCall.Release()

	// Set up the keypress handler
	renderActive = atomic.NewBool(false)
	kCall = js.NewCallback(keypressHandler)
	doc.Call("addEventListener", "keydown", kCall)
	defer kCall.Release()

	// Set up the mouse move handler
	mCall = js.NewCallback(moveHandler)
	doc.Call("addEventListener", "mousemove", mCall)
	defer mCall.Release()

	// Set the frame renderer going
	rCall = js.NewCallback(renderFrame)
	js.Global().Call("requestAnimationFrame", rCall)
	defer rCall.Release()

	// Set up the mouse wheel handler
	wCall = js.NewCallback(wheelHandler)
	doc.Call("addEventListener", "wheel", wCall)
	defer wCall.Release()

	// Set the operations processor going
	queue = make(chan Operation)
	go processOperations(queue)

	// Add the X/Y axes object to the world space
	worldSpace = append(worldSpace, importObject(axes, 0.0, 0.0, 0.0))

	// Create a graph object with the main data points on it
	// TODO: Allow user input of equation to graph
	//eqStr = "x^2"
	eqStr = "x^3"
	//eqStr = "x^4" // Eventually does display stuff, but takes about 1 minute before anything appears
	//eqStr = "(x^3)/2"
	//eqStr = "(3/2)*x^2"
	var graph Object
	var p Point
	errOccurred := false
	graphLabeled := false
	evalState := eq.NewEvalState()
	expr := eq.Interp(fmt.Sprintf("f[x_] := %s", eqStr), evalState)
	result := expr.Eval(evalState)
	result = evalState.ProcessTopLevelResult(expr, result)
	for x := -2.1; x <= 2.1; x += 0.05 {
		expr = eq.Interp(fmt.Sprintf("x=%.2f", x), evalState)
		result := expr.Eval(evalState)
		result = evalState.ProcessTopLevelResult(expr, result)
		expr = eq.Interp("f[x]", evalState)
		result = expr.Eval(evalState)
		result = evalState.ProcessTopLevelResult(expr, result)
		y, err := strconv.ParseFloat(result.StringForm(eq.ActualStringFormArgsFull("InputForm", evalState)), 64)
		if err != nil {
			y = -1 // Set this to -1 to visually indicate something went wrong
			errOccurred = true
			fmt.Printf("Error: %v\n", err)
		}
		p = Point{X: x, Y: y}
		if !graphLabeled {
			p.Label = fmt.Sprintf(" Equation: y = %s ", mathFormat(eqStr))
			p.LabelAlign = "right"
			graphLabeled = true
		}
		graph.P = append(graph.P, p)
	}
	if errOccurred {
		graph.C = "red" // Draw the line in red if an error occurred with the calculation
	} else {
		graph.C = "blue"
	}
	graph.Name = "Equation"
	graph.Eq = fmt.Sprintf("y = %s", mathFormat(eqStr))
	worldSpace = append(worldSpace, importObject(graph, 0.0, 0.0, 0.0))

	// Graph the derivatives of the equation
	derivNum := 1
	straightLine := true
	for derivNum == 1 || straightLine != true { // Make sure at least one derivative gets calculated
		straightLine = true // The slope check further on will toggle this back off if the derivative isn't a straight line

		// Retrieve the human readable string for the derivative
		tmpStr := fmt.Sprintf("D[%s, x]", eqStr)
		tmpState := eq.NewEvalState()
		tmpExpr := eq.Interp(tmpStr, tmpState)
		tmpResult := tmpExpr.Eval(tmpState)
		tmpResult = tmpState.ProcessTopLevelResult(tmpExpr, tmpResult)
		derivStr = tmpResult.StringForm(eq.ActualStringFormArgsFull("OutputForm", tmpState))

		// Variables used to determine if the derivative is a straight line
		gotSlope := false
		gotFirstPoint := false
		var slope, slope2, slopeP1, slopeP2 float64

		// Create a graph object with the derivative points on it
		errOccurred = false
		graphLabeled = false
		derivState := eq.NewEvalState()
		var deriv Object
		var derivExpr, derivResult eq.Ex
		for x := -2.1; x <= 2.1; x += pointStep {
			derivEq := fmt.Sprintf("D[%s,x] /. x -> %.2f", eqStr, x)
			derivExpr = eq.Interp(derivEq, derivState)
			derivResult = derivExpr.Eval(derivState)
			derivResult = derivState.ProcessTopLevelResult(derivExpr, derivResult)
			tmp := derivResult.StringForm(eq.ActualStringFormArgsFull("OutputForm", derivState))
			if debug {
				fmt.Printf("Val: %0.2f Derivative String: %v Result: %v\n", x, derivStr, tmp)
			}
			y, err := strconv.ParseFloat(tmp, 64)
			if err != nil {
				y = -1 // Set this to -1 to visually indicate something went wrong
				errOccurred = true
				fmt.Printf("Error: %v\n", err)
			}

			// Determine if the derivative is a straight line
			if !gotSlope {
				if !gotFirstPoint {
					slopeP1 = math.Round(y*10000) / 10000 // Round off, but keep a few decimal places of precision
					gotFirstPoint = true
				} else {
					slopeP2 = math.Round(y*10000) / 10000
					riseOverRun := (slopeP2 - slopeP1) / pointStep
					slope = math.Round(riseOverRun*10000) / 10000
					if debug {
						fmt.Printf("Slope: (%v - %v) / %v = %v\n", slopeP2, slopeP1, pointStep, slope)
					}
					slopeP1 = slopeP2
					gotSlope = true
				}
			} else {
				slopeP2 = math.Round(y*10000) / 10000
				riseOverRun := (slopeP2 - slopeP1) / pointStep
				slope2 = math.Round(riseOverRun*10000) / 10000
				if debug {
					fmt.Printf("Slope2: (%v - %v) / %v = %v", slopeP2, slopeP1, pointStep, slope2)
				}
				slopeP1 = slopeP2
				if slope != slope2 {
					straightLine = false
				}
				if debug {
					fmt.Printf(" Straight line: %v\n", straightLine)
				}
			}

			p = Point{X: x, Y: y}
			if !graphLabeled {
				p.Label = fmt.Sprintf(" %s order derivative: y = %s ", strDeriv(derivNum), mathFormat(derivStr))
				p.LabelAlign = "right"
				graphLabeled = true
			}
			deriv.P = append(deriv.P, p)
		}
		if errOccurred {
			deriv.C = "red" // Draw the line in red if an error occurred with the calculation
		} else {
			deriv.C = colDeriv(derivNum)
		}
		deriv.Name = fmt.Sprintf("%s order derivative", strDeriv(derivNum))
		deriv.Eq = fmt.Sprintf("y = %s", mathFormat(derivStr))
		worldSpace = append(worldSpace, importObject(deriv, 0.0, 0.0, 0.0))
		eqStr = derivStr
		derivNum++
	}

	// Keep the application running
	done := make(chan struct{}, 0)
	<-done
}

// Simple mouse handler watching for people clicking on the source code link
func clickHandler(args []js.Value) {
	event := args[0]
	clientX := event.Get("clientX").Float()
	clientY := event.Get("clientY").Float()
	if debug {
		fmt.Printf("ClientX: %v  clientY: %v\n", clientX, clientY)
		if clientX > graphWidth && clientY > (height-40) {
			println("URL hit!")
		}
	}

	// If the user clicks the source code URL area, open the URL
	if clientX > graphWidth && clientY > (height-40) {
		w := js.Global().Call("open", sourceURL)
		if w == js.Null() {
			// Couldn't open a new window, so try loading directly in the existing one instead
			doc.Set("location", sourceURL)
		}
	}
}

// Returns the colour to use for a derivative
func colDeriv(i int) string {
	switch i {
	case 1:
		return "green"
	case 2:
		return "darkgoldenrod"
	case 3:
		return "chocolate"
	default:
		return "black"
	}
}

// Returns an object whose points have been transformed into 3D world space XYZ co-ordinates.  Also assigns a number
// to each point
func importObject(ob Object, x float64, y float64, z float64) (translatedObject Object) {
	// X and Y translation matrix.  Translates the objects into the world space at the given X and Y co-ordinates
	translateMatrix := matrix{
		1, 0, 0, x,
		0, 1, 0, y,
		0, 0, 1, z,
		0, 0, 0, 1,
	}

	// Translate the points
	var pt Point
	for _, j := range ob.P {
		pt = Point{
			Label:      j.Label,
			LabelAlign: j.LabelAlign,
			X:          (translateMatrix[0] * j.X) + (translateMatrix[1] * j.Y) + (translateMatrix[2] * j.Z) + (translateMatrix[3] * 1),   // 1st col, top
			Y:          (translateMatrix[4] * j.X) + (translateMatrix[5] * j.Y) + (translateMatrix[6] * j.Z) + (translateMatrix[7] * 1),   // 1st col, upper middle
			Z:          (translateMatrix[8] * j.X) + (translateMatrix[9] * j.Y) + (translateMatrix[10] * j.Z) + (translateMatrix[11] * 1), // 1st col, lower middle
		}
		translatedObject.P = append(translatedObject.P, pt)
	}

	// Copy the remaining object info across
	translatedObject.C = ob.C
	translatedObject.Name = ob.Name
	translatedObject.Eq = ob.Eq
	for _, j := range ob.E {
		translatedObject.E = append(translatedObject.E, j)
	}
	for _, j := range ob.S {
		translatedObject.S = append(translatedObject.S, j)
	}

	return translatedObject
}

// Simple keyboard handler for catching the arrow, WASD, and numpad keys
// Key value info can be found here: https://developer.mozilla.org/en-US/docs/Web/API/KeyboardEvent/key/Key_Values
func keypressHandler(args []js.Value) {
	event := args[0]
	key := event.Get("key").String()
	if debug {
		fmt.Printf("Key is: %v\n", key)
	}

	// Don't add operations if one is already in progress
	stepSize := float64(25)
	if !renderActive.Load() {
		switch key {
		case "ArrowLeft", "a", "A", "4":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: 0, Y: -stepSize, Z: 0}
		case "ArrowRight", "d", "D", "6":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: 0, Y: stepSize, Z: 0}
		case "ArrowUp", "w", "W", "8":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: -stepSize, Y: 0, Z: 0}
		case "ArrowDown", "s", "S", "2":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: stepSize, Y: 0, Z: 0}
		case "7", "Home":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: -stepSize, Y: -stepSize, Z: 0}
		case "9", "PageUp":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: -stepSize, Y: stepSize, Z: 0}
		case "1", "End":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: stepSize, Y: -stepSize, Z: 0}
		case "3", "PageDown":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: stepSize, Y: stepSize, Z: 0}
		case "-":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: 0, Y: 0, Z: -stepSize}
		case "+":
			queue <- Operation{op: ROTATE, t: 50, f: 12, X: 0, Y: 0, Z: stepSize}
		}
	}
}

// Pretty formatting of maths strings.  Changes (say) x^3 to x³
func mathFormat(s string) string {
	numFind := regexp.MustCompile(`\^[0-9]+`)
	numFind.Longest()
	t := numFind.ReplaceAllStringFunc(s, func(t string) string {
		var u strings.Builder
		for _, j := range t {
			switch j {
			case '0':
				u.WriteString("⁰")
			case '1':
				u.WriteString("¹")
			case '2':
				u.WriteString("²")
			case '3':
				u.WriteString("³")
			case '4':
				u.WriteString("⁴")
			case '5':
				u.WriteString("⁵")
			case '6':
				u.WriteString("⁶")
			case '7':
				u.WriteString("⁷")
			case '8':
				u.WriteString("⁸")
			case '9':
				u.WriteString("⁹")
			}
		}

		return u.String()
	})
	return t
}

// Multiplies one matrix by another
func matrixMult(opMatrix matrix, m matrix) (resultMatrix matrix) {
	top0 := m[0]
	top1 := m[1]
	top2 := m[2]
	top3 := m[3]
	upperMid0 := m[4]
	upperMid1 := m[5]
	upperMid2 := m[6]
	upperMid3 := m[7]
	lowerMid0 := m[8]
	lowerMid1 := m[9]
	lowerMid2 := m[10]
	lowerMid3 := m[11]
	bot0 := m[12]
	bot1 := m[13]
	bot2 := m[14]
	bot3 := m[15]

	resultMatrix = matrix{
		(opMatrix[0] * top0) + (opMatrix[1] * upperMid0) + (opMatrix[2] * lowerMid0) + (opMatrix[3] * bot0), // 1st col, top
		(opMatrix[0] * top1) + (opMatrix[1] * upperMid1) + (opMatrix[2] * lowerMid1) + (opMatrix[3] * bot1), // 2nd col, top
		(opMatrix[0] * top2) + (opMatrix[1] * upperMid2) + (opMatrix[2] * lowerMid2) + (opMatrix[3] * bot2), // 3rd col, top
		(opMatrix[0] * top3) + (opMatrix[1] * upperMid3) + (opMatrix[2] * lowerMid3) + (opMatrix[3] * bot3), // 4th col, top

		(opMatrix[4] * top0) + (opMatrix[5] * upperMid0) + (opMatrix[6] * lowerMid0) + (opMatrix[7] * bot0), // 1st col, upper middle
		(opMatrix[4] * top1) + (opMatrix[5] * upperMid1) + (opMatrix[6] * lowerMid1) + (opMatrix[7] * bot1), // 2nd col, upper middle
		(opMatrix[4] * top2) + (opMatrix[5] * upperMid2) + (opMatrix[6] * lowerMid2) + (opMatrix[7] * bot2), // 3rd col, upper middle
		(opMatrix[4] * top3) + (opMatrix[5] * upperMid3) + (opMatrix[6] * lowerMid3) + (opMatrix[7] * bot3), // 4th col, upper middle

		(opMatrix[8] * top0) + (opMatrix[9] * upperMid0) + (opMatrix[10] * lowerMid0) + (opMatrix[11] * bot0), // 1st col, lower middle
		(opMatrix[8] * top1) + (opMatrix[9] * upperMid1) + (opMatrix[10] * lowerMid1) + (opMatrix[11] * bot1), // 2nd col, lower middle
		(opMatrix[8] * top2) + (opMatrix[9] * upperMid2) + (opMatrix[10] * lowerMid2) + (opMatrix[11] * bot2), // 3rd col, lower middle
		(opMatrix[8] * top3) + (opMatrix[9] * upperMid3) + (opMatrix[10] * lowerMid3) + (opMatrix[11] * bot3), // 4th col, lower middle

		(opMatrix[12] * top0) + (opMatrix[13] * upperMid0) + (opMatrix[14] * lowerMid0) + (opMatrix[15] * bot0), // 1st col, bottom
		(opMatrix[12] * top1) + (opMatrix[13] * upperMid1) + (opMatrix[14] * lowerMid1) + (opMatrix[15] * bot1), // 2nd col, bottom
		(opMatrix[12] * top2) + (opMatrix[13] * upperMid2) + (opMatrix[14] * lowerMid2) + (opMatrix[15] * bot2), // 3rd col, bottom
		(opMatrix[12] * top3) + (opMatrix[13] * upperMid3) + (opMatrix[14] * lowerMid3) + (opMatrix[15] * bot3), // 4th col, bottom
	}
	return resultMatrix
}

// Simple mouse handler watching for people moving the mouse over the source code link
func moveHandler(args []js.Value) {
	event := args[0]
	clientX := event.Get("clientX").Float()
	clientY := event.Get("clientY").Float()
	if debug {
		fmt.Printf("ClientX: %v  clientY: %v\n", clientX, clientY)
	}

	// If the mouse is over the source code link, let the frame renderer know to draw the url in bold
	if clientX > graphWidth && clientY > (height-40) {
		highLightSource = true
	} else {
		highLightSource = false
	}
}

// Animates the transformation operations
func processOperations(queue <-chan Operation) {
	for i := range queue {
		renderActive.Store(true)         // Mark rendering as now in progress
		parts := i.f                     // Number of parts to break each transformation into
		transformMatrix = identityMatrix // Reset the transform matrix
		switch i.op {
		case ROTATE: // Rotate the objects in world space
			// Divide the desired angle into a small number of parts
			if i.X != 0 {
				transformMatrix = rotateAroundX(transformMatrix, i.X/float64(parts))
			}
			if i.Y != 0 {
				transformMatrix = rotateAroundY(transformMatrix, i.Y/float64(parts))
			}
			if i.Z != 0 {
				transformMatrix = rotateAroundZ(transformMatrix, i.Z/float64(parts))
			}
			opText = fmt.Sprintf("Rotation. X: %0.2f Y: %0.2f Z: %0.2f", i.X, i.Y, i.Z)

		case SCALE:
			// Scale the objects in world space
			var xPart, yPart, zPart float64
			if i.X != 1 {
				xPart = ((i.X - 1) / float64(parts)) + 1
			}
			if i.Y != 1 {
				yPart = ((i.Y - 1) / float64(parts)) + 1
			}
			if i.Z != 1 {
				zPart = ((i.Z - 1) / float64(parts)) + 1
			}
			transformMatrix = scale(transformMatrix, xPart, yPart, zPart)
			opText = fmt.Sprintf("Scale. X: %0.2f Y: %0.2f Z: %0.2f", i.X, i.Y, i.Z)

		case TRANSLATE:
			// Translate (move) the objects in world space
			transformMatrix = translate(transformMatrix, i.X/float64(parts), i.Y/float64(parts), i.Z/float64(parts))
			opText = fmt.Sprintf("Translate (move). X: %0.2f Y: %0.2f Z: %0.2f", i.X, i.Y, i.Z)
		}

		// Apply each transformation, one small part at a time (this gives the animation effect)
		timeSlice := time.Millisecond * time.Duration(i.t/parts)
		for t := 0; t < int(parts); t++ {
			time.Sleep(timeSlice)
			for j, o := range worldSpace {
				var newPoints []Point

				// Transform each point of in the object
				for _, j := range o.P {
					newPoints = append(newPoints, transform(transformMatrix, j))
				}
				o.P = newPoints

				// Update the object in world space
				worldSpace[j] = o
			}
		}
		renderActive.Store(false)
		opText = "Complete."
	}
}

// Renders one frame of the animation
func renderFrame(args []js.Value) {
	// Handle window resizing
	curBodyW := doc.Get("body").Get("clientWidth").Float()
	curBodyH := doc.Get("body").Get("clientHeight").Float()
	if curBodyW != width || curBodyH != height {
		width, height = curBodyW, curBodyH
		canvasEl.Set("width", width)
		canvasEl.Set("height", height)
	}

	// Setup useful variables
	border := float64(2)
	gap := float64(3)
	left := border + gap
	top := border + gap
	graphWidth = width * 0.75
	graphHeight = height - 1
	centerX := graphWidth / 2
	centerY := graphHeight / 2

	// Clear the background
	ctx.Set("fillStyle", "white")
	ctx.Call("fillRect", 0, 0, width, height)

	// Draw grid lines
	step := math.Min(width, height) / 30
	ctx.Set("strokeStyle", "rgb(220, 220, 220)")
	ctx.Call("setLineDash", []interface{}{1, 3})
	for i := left; i < graphWidth-step; i += step {
		// Vertical dashed lines
		ctx.Call("beginPath")
		ctx.Call("moveTo", i+step, top)
		ctx.Call("lineTo", i+step, graphHeight)
		ctx.Call("stroke")
	}
	for i := top; i < graphHeight-step; i += step {
		// Horizontal dashed lines
		ctx.Call("beginPath")
		ctx.Call("moveTo", left, i+step)
		ctx.Call("lineTo", graphWidth-border, i+step)
		ctx.Call("stroke")
	}

	// Draw the axes
	var pointX, pointY float64
	ctx.Set("strokeStyle", "black")
	ctx.Set("lineWidth", "1")
	ctx.Call("setLineDash", []interface{}{})
	for _, o := range worldSpace {
		// Draw the surfaces
		ctx.Set("fillStyle", o.C)
		for _, l := range o.S {
			for m, n := range l {
				pointX = o.P[n].X
				pointY = o.P[n].Y
				if m == 0 {
					ctx.Call("beginPath")
					ctx.Call("moveTo", centerX+(pointX*step), centerY+((pointY*step)*-1))
				} else {
					ctx.Call("lineTo", centerX+(pointX*step), centerY+((pointY*step)*-1))
				}
			}
			ctx.Call("closePath")
			ctx.Call("fill")
		}

		// Draw the edges
		var point1X, point1Y, point2X, point2Y float64
		for _, l := range o.E {
			point1X = o.P[l[0]].X
			point1Y = o.P[l[0]].Y
			point2X = o.P[l[1]].X
			point2Y = o.P[l[1]].Y
			ctx.Call("beginPath")
			ctx.Call("moveTo", centerX+(point1X*step), centerY+((point1Y*step)*-1))
			ctx.Call("lineTo", centerX+(point2X*step), centerY+((point2Y*step)*-1))
			ctx.Call("stroke")
		}

		// Draw any point labels
		ctx.Set("fillStyle", "black")
		ctx.Set("font", "bold 16px serif")
		var px, py float64
		for _, l := range o.P {
			if l.Label != "" {
				ctx.Set("textAlign", l.LabelAlign)
				px = centerX + (l.X * step)
				py = centerY + ((l.Y * step) * -1)
				ctx.Call("fillText", l.Label, px, py)
			}
		}
	}

	// Draw the graph and derivatives
	ctx.Set("lineWidth", "2")
	ctx.Call("setLineDash", []interface{}{})
	var px, py float64
	numWld := len(worldSpace)
	for i := 0; i < numWld; i++ {
		o := worldSpace[i]
		if o.Name != "axes" {
			// Draw lines between the points
			ctx.Set("strokeStyle", o.C)
			ctx.Call("beginPath")
			for k, l := range o.P {
				px = centerX + (l.X * step)
				py = centerY + ((l.Y * step) * -1)
				if k == 0 {
					ctx.Call("moveTo", px, py)
				} else {
					ctx.Call("lineTo", px, py)
				}
			}
			ctx.Call("stroke")

			// Draw dots for the points
			ctx.Set("fillStyle", "black")
			for _, l := range o.P {
				px = centerX + (l.X * step)
				py = centerY + ((l.Y * step) * -1)
				ctx.Call("beginPath")
				ctx.Call("ellipse", px, py, 1, 1, 0, 0, 2*math.Pi)
				ctx.Call("fill")
				ctx.Call("stroke")
			}
		}
	}

	// Clear the information area (right side)
	ctx.Set("fillStyle", "white")
	ctx.Call("fillRect", graphWidth+1, 0, width, height)

	// Draw the text describing the current operation
	textY := top + 20
	ctx.Set("fillStyle", "black")
	ctx.Set("font", "bold 14px serif")
	ctx.Set("textAlign", "left")
	ctx.Call("fillText", "Operation:", graphWidth+20, textY)
	textY += 20
	ctx.Set("font", "14px sans-serif")
	ctx.Call("fillText", opText, graphWidth+20, textY)
	textY += 30

	// Add the help text about control keys and mouse zoom
	ctx.Set("fillStyle", "blue")
	ctx.Set("font", "14px sans-serif")
	ctx.Call("fillText", "Use wasd/numpad keys to rotate,", graphWidth+20, textY)
	textY += 20
	ctx.Call("fillText", "mouse wheel to zoom.", graphWidth+20, textY)
	textY += 30

	// Add the graph and derivatives information
	ctx.Set("fillStyle", "black")
	for i := 0; i < numWld; i++ {
		o := worldSpace[i]
		if o.Name != "axes" {
			ctx.Set("font", "bold 18px serif")
			ctx.Call("fillText", o.Name, graphWidth+20, textY)
			textY += 20
			ctx.Set("font", "16px sans-serif")
			ctx.Call("fillText", o.Eq, graphWidth+40, textY)
			textY += 30
		}
	}

	// Clear the source code link area
	ctx.Set("fillStyle", "white")
	ctx.Call("fillRect", graphWidth+1, graphHeight-55, width, height)

	// Add the URL to the source code
	ctx.Set("fillStyle", "black")
	ctx.Set("font", "bold 14px serif")
	ctx.Call("fillText", "Source code:", graphWidth+20, graphHeight-35)
	ctx.Set("fillStyle", "blue")
	if highLightSource == true {
		ctx.Set("font", "bold 12px sans-serif")
	} else {
		ctx.Set("font", "12px sans-serif")
	}
	ctx.Call("fillText", sourceURL, graphWidth+20, graphHeight-15)

	// Draw a border around the graph area
	ctx.Call("setLineDash", []interface{}{})
	ctx.Set("lineWidth", "2")
	ctx.Set("strokeStyle", "white")
	ctx.Call("beginPath")
	ctx.Call("moveTo", 0, 0)
	ctx.Call("lineTo", width, 0)
	ctx.Call("lineTo", width, height)
	ctx.Call("lineTo", 0, height)
	ctx.Call("closePath")
	ctx.Call("stroke")
	ctx.Set("lineWidth", "2")
	ctx.Set("strokeStyle", "black")
	ctx.Call("beginPath")
	ctx.Call("moveTo", border, border)
	ctx.Call("lineTo", graphWidth, border)
	ctx.Call("lineTo", graphWidth, graphHeight)
	ctx.Call("lineTo", border, graphHeight)
	ctx.Call("closePath")
	ctx.Call("stroke")

	// Schedule the next frame render call
	js.Global().Call("requestAnimationFrame", rCall)
}

// Rotates a transformation matrix around the X axis by the given degrees
func rotateAroundX(m matrix, degrees float64) matrix {
	rad := (math.Pi / 180) * degrees // The Go math functions use radians, so we convert degrees to radians
	rotateXMatrix := matrix{
		1, 0, 0, 0,
		0, math.Cos(rad), -math.Sin(rad), 0,
		0, math.Sin(rad), math.Cos(rad), 0,
		0, 0, 0, 1,
	}
	return matrixMult(rotateXMatrix, m)
}

// Rotates a transformation matrix around the Y axis by the given degrees
func rotateAroundY(m matrix, degrees float64) matrix {
	rad := (math.Pi / 180) * degrees // The Go math functions use radians, so we convert degrees to radians
	rotateYMatrix := matrix{
		math.Cos(rad), 0, math.Sin(rad), 0,
		0, 1, 0, 0,
		-math.Sin(rad), 0, math.Cos(rad), 0,
		0, 0, 0, 1,
	}
	return matrixMult(rotateYMatrix, m)
}

// Rotates a transformation matrix around the Z axis by the given degrees
func rotateAroundZ(m matrix, degrees float64) matrix {
	rad := (math.Pi / 180) * degrees // The Go math functions use radians, so we convert degrees to radians
	rotateZMatrix := matrix{
		math.Cos(rad), -math.Sin(rad), 0, 0,
		math.Sin(rad), math.Cos(rad), 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
	return matrixMult(rotateZMatrix, m)
}

// Scales a transformation matrix by the given X, Y, and Z values
func scale(m matrix, x float64, y float64, z float64) matrix {
	scaleMatrix := matrix{
		x, 0, 0, 0,
		0, y, 0, 0,
		0, 0, z, 0,
		0, 0, 0, 1,
	}
	return matrixMult(scaleMatrix, m)
}

// Returns the name/label prefix for a derivative string
func strDeriv(i int) string {
	switch i {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	default:
		return fmt.Sprintf("%dth", i)
	}
}

// Transform the XYZ co-ordinates using the values from the transformation matrix
func transform(m matrix, p Point) (t Point) {
	top0 := m[0]
	top1 := m[1]
	top2 := m[2]
	top3 := m[3]
	upperMid0 := m[4]
	upperMid1 := m[5]
	upperMid2 := m[6]
	upperMid3 := m[7]
	lowerMid0 := m[8]
	lowerMid1 := m[9]
	lowerMid2 := m[10]
	lowerMid3 := m[11]
	//bot0 := m[12] // The fourth row values can be ignored for 3D matrices
	//bot1 := m[13]
	//bot2 := m[14]
	//bot3 := m[15]

	t.Label = p.Label
	t.LabelAlign = p.LabelAlign
	t.X = (top0 * p.X) + (top1 * p.Y) + (top2 * p.Z) + top3
	t.Y = (upperMid0 * p.X) + (upperMid1 * p.Y) + (upperMid2 * p.Z) + upperMid3
	t.Z = (lowerMid0 * p.X) + (lowerMid1 * p.Y) + (lowerMid2 * p.Z) + lowerMid3
	return
}

// Translates (moves) a transformation matrix by the given X, Y and Z values
func translate(m matrix, translateX float64, translateY float64, translateZ float64) matrix {
	translateMatrix := matrix{
		1, 0, 0, translateX,
		0, 1, 0, translateY,
		0, 0, 1, translateZ,
		0, 0, 0, 1,
	}
	return matrixMult(translateMatrix, m)
}

// Simple mouse handler watching for mouse wheel events
// Reference info can be found here: https://developer.mozilla.org/en-US/docs/Web/Events/wheel
func wheelHandler(args []js.Value) {
	event := args[0]
	wheelDelta := event.Get("deltaY").Float()
	scaleSize := 1 + (wheelDelta / 5)
	if debug {
		fmt.Printf("Wheel delta: %v, scaleSize: %v\n", wheelDelta, scaleSize)
	}

	// Don't add operations if one is already in progress
	if !renderActive.Load() {
		queue <- Operation{op: SCALE, t: 50, f: 12, X: scaleSize, Y: scaleSize, Z: scaleSize}
	}
}
