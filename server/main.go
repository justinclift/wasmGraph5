package main

// A simple server to return derivatives and basic solved equations to the wasmGraph5 front end

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	eq "github.com/corywalker/expreduce/expreduce"
	"github.com/husobee/vestigo"
)

type Point struct {
	X, Y float64
}

var (
	debug = true // If true, some debugging info is printed to the javascript console
)

func main() {
	router := vestigo.NewRouter()
	// TODO: Learn what these CORS settings actually do
	router.SetGlobalCors(&vestigo.CorsAccessControl{
		AllowOrigin:      []string{"*"},
		AllowCredentials: true,
		ExposeHeaders:    []string{"X-Header", "X-Y-Header"},
		MaxAge:           3600 * time.Second,
		AllowHeaders:     []string{"X-Header", "X-Y-Header"},
	})
	router.Get("/derivstr/", derivStrHandler)
	router.Get("/solvederiv/", solveDerivHandler)
	router.Get("/solveeq/", solveEqHandler)
	err := http.ListenAndServe("0.0.0.0:8080", router)
	if err != nil {
		log.Fatal(err)
	}
}

// Returns the derivative string for a given input equation string
//   * Use a browser to simulate a call with (eg): http://0.0.0.0:8080/derivstr/?eq=x^2
func derivStrHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential equation string
	inp := r.FormValue("eq")
	if debug {
		fmt.Printf("Derivative input '%v' received\n", inp)
	}

	// Input validation
	//newEq, err := validate(inp)
	//if err != nil {
	//	http.Error(w, err.Error(), http.StatusBadRequest)
	//
	//	// Display message on server console
	//	if debug {
	//		fmt.Println(err.Error())
	//	}
	//	return
	//}

	str := "x^2"

	// Return the derivative of the equation
	//str := fmt.Sprintf("D[%s, x]", newEq.String())
	state := eq.NewEvalState()
	expr := eq.Interp(str, state)
	result := expr.Eval(state)
	result = state.ProcessTopLevelResult(expr, result)
	deriv := result.StringForm(eq.ActualStringFormArgsFull("OutputForm", state))
	io.WriteString(w, deriv)

	// Print the equation and derivative to the server console
	//if debug {
	//	fmt.Printf("Equation: '%v' - Derivative: '%v'\n", newEq.String(), deriv)
	//}
}

// Returns the derivative for a given input equation string + value
//   * Use a browser to simulate a call with (eg): http://0.0.0.0:8080/solvederiv/?eq=x^2&min=-2.0&max=2.1&step=0.1
func solveDerivHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the inputs
	inpEq := r.FormValue("eq")
	inpMax := r.FormValue("max")
	inpMin := r.FormValue("min")
	inpStep := r.FormValue("step")
	if debug {
		fmt.Printf("Derivative input:\n * Eq: '%v'\n * Min: '%v'\n * Max: '%v'\n * Step: '%v'\n", inpEq, inpMin,
			inpMax, inpStep)
	}

	// Validate the inputs
	newEq, min, max, step, err := validate(inpEq, inpMin, inpMax, inpStep)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		if debug {
			fmt.Println(err.Error())
		}
		return
	}

	// Solve the derivative equation
	var points []Point
	var errOccurred error
	var derivExpr, derivResult eq.Ex
	derivState := eq.NewEvalState()
	for x := min; x <= max; x += step {
		derivEq := fmt.Sprintf("D[%s,x] /. x -> %.2f", newEq, x)
		derivExpr = eq.Interp(derivEq, derivState)
		derivResult = derivExpr.Eval(derivState)
		derivResult = derivState.ProcessTopLevelResult(derivExpr, derivResult)
		tmp := derivResult.StringForm(eq.ActualStringFormArgsFull("OutputForm", derivState))
		if debug {
			fmt.Printf("Val: %0.2f Derivative String: %v Result: %v\n", x, newEq, tmp)
		}
		y, err := strconv.ParseFloat(tmp, 64)
		if err != nil {
			y = -1 // Set this to -1 to visually indicate something went wrong
			errOccurred = err
			fmt.Printf("Error: %v\n", err)
		}
		points = append(points, Point{X: x, Y: y})
	}
	if errOccurred != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	// Return the result as JSON
	output, err := json.MarshalIndent(points, "", " ")
	if err != nil {
		log.Println(err)
		return
	}
	io.WriteString(w, string(output))

	// Print the equation, value and result to the server console
	if debug {
		fmt.Printf("Derivative equation: '%v'\nResult: '%s'\n", newEq, output)
	}
}

// Returns the solved equation for a given input equation string + value
//   * Use a browser to simulate a call with (eg): http://0.0.0.0:8080/solveeq/?eq=x^2&val=2.0
func solveEqHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential equation string and value
	//inpEq := r.FormValue("eq")
	//inpMax := r.FormValue("max")
	//inpMin := r.FormValue("min")
	//inpStep := r.FormValue("step")

	//if debug {
	//	fmt.Printf("Solve input: equation '%v' with value '%v' received\n", inpEq, inpVal)
	//}

	//// Validate equation string
	//newEq, err := validate(inpEq)
	//if err != nil {
	//	http.Error(w, err.Error(), http.StatusBadRequest)
	//	if debug {
	//		fmt.Println(err.Error())
	//	}
	//	return
	//}
	//
	//// Validate floating point value string
	//val, err := strconv.ParseFloat(inpVal, 64)
	//if err != nil {
	//	http.Error(w, err.Error(), http.StatusBadRequest)
	//	if debug {
	//		fmt.Println(err.Error())
	//	}
	//	return
	//}

	str := "x^2"
	val := 2.0

	// Solve the equation
	//str := fmt.Sprintf("f[x_] := %s", newEq.String())
	state := eq.NewEvalState()
	expr := eq.Interp(str, state)
	part1 := expr.Eval(state)
	part1 = state.ProcessTopLevelResult(expr, part1)
	expr = eq.Interp(fmt.Sprintf("x=%.2f", val), state)
	part2 := expr.Eval(state)
	part2 = state.ProcessTopLevelResult(expr, part2)
	expr = eq.Interp("f[x]", state)
	part2 = expr.Eval(state)
	part2 = state.ProcessTopLevelResult(expr, part2)
	result := part2.StringForm(eq.ActualStringFormArgsFull("InputForm", state))
	io.WriteString(w, result)

	// Print the equation, value and result to the server console
	//if debug {
	//	fmt.Printf("Equation: '%v', value: '%v' - Result: '%v'\n", newEq.String(), val, result)
	//}
}

// Validates inputs
func validate(inpEq string, inpMin string, inpMax string, inpStep string) (newEq string, min float64, max float64, step float64, err error) {
	var badChars strings.Builder
	var s strings.Builder
	for _, j := range inpEq {
		switch j {
		case 'x', '+', '-', '*', '/', '^', ' ', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '(', ')', '[', ']':
			// The character is valid
			s.WriteRune(j)
		default:
			// The character is NOT valid
			badChars.WriteRune(j)
		}
	}
	if badChars.String() != "" {
		// Return an appropriate error message
		err = fmt.Errorf("bad character(s) in equation input string: %s", badChars.String())
		return
	}
	newEq = s.String()

	// Validate floating point values
	min, err = strconv.ParseFloat(inpMin, 64)
	if err != nil {
		return
	}
	max, err = strconv.ParseFloat(inpMax, 64)
	if err != nil {
		return
	}
	step, err = strconv.ParseFloat(inpStep, 64)
	if err != nil {
		return
	}
	return
}
