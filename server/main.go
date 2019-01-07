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

// Returns the derivative formula for a given input equation
//   * Use a browser to simulate a call with (eg): http://0.0.0.0:8080/derivstr/?eq=x^2
func derivStrHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential equation string
	inp := r.FormValue("eq")
	if debug {
		fmt.Printf("Derivative input '%v' received\n", inp)
	}

	// Input validation
	newEq, err := validateEqStr(inp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		// Display message on server console
		if debug {
			fmt.Println(err.Error())
		}
		return
	}

	// Return the derivative formula for the equation
	str := fmt.Sprintf("D[%s, x]", newEq)
	state := eq.NewEvalState()
	expr := eq.Interp(str, state)
	result := expr.Eval(state)
	result = state.ProcessTopLevelResult(expr, result)
	deriv := result.StringForm(eq.ActualStringFormArgsFull("OutputForm", state))
	_, err = io.WriteString(w, deriv)
	if err != nil {
		fmt.Println(err.Error())
	}

	// Print the equation and derivative to the server console
	if debug {
		fmt.Printf("Equation: '%v' - Derivative: '%v'\n", newEq, deriv)
	}
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
		return
	}

	// Return the result as JSON
	output, err := json.MarshalIndent(points, "", " ")
	if err != nil {
		log.Println(err)
		return
	}
	_, err = io.WriteString(w, string(output))
	if err != nil {
		fmt.Println(err.Error())
	}

	// Print the equation, value and result to the server console
	if debug {
		fmt.Printf("Derivative equation: '%v'\nResult: '%s'\n", newEq, output)
	}
}

// Returns the solved equation for a given input equation string + value
//   * Use a browser to simulate a call with (eg): http://0.0.0.0:8080/solveeq/?eq=x^2&min=-2.0&max=2.1&step=0.1
func solveEqHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential equation string and value
	inpEq := r.FormValue("eq")
	inpMax := r.FormValue("max")
	inpMin := r.FormValue("min")
	inpStep := r.FormValue("step")
	if debug {
		fmt.Printf("Solve input:\n * Eq: '%v'\n * Min: '%v'\n * Max: '%v'\n * Step: '%v'\n", inpEq, inpMin,
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

	// Solve the equation
	var points []Point
	var errOccurred error
	evalState := eq.NewEvalState()
	expr := eq.Interp(fmt.Sprintf("f[x_] := %s", newEq), evalState)
	result := expr.Eval(evalState)
	result = evalState.ProcessTopLevelResult(expr, result)
	for x := min; x <= max; x += step {
		expr = eq.Interp(fmt.Sprintf("x=%.2f", x), evalState)
		result := expr.Eval(evalState)
		result = evalState.ProcessTopLevelResult(expr, result)
		expr = eq.Interp("f[x]", evalState)
		result = expr.Eval(evalState)
		result = evalState.ProcessTopLevelResult(expr, result)
		y, err := strconv.ParseFloat(result.StringForm(eq.ActualStringFormArgsFull("InputForm", evalState)), 64)
		if err != nil {
			y = -1 // Set this to -1 to visually indicate something went wrong
			errOccurred = err
			fmt.Printf("Error: %v\n", err)
		}
		points = append(points, Point{X: x, Y: y})
	}
	if errOccurred != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return the result as JSON
	output, err := json.MarshalIndent(points, "", " ")
	if err != nil {
		log.Println(err)
		return
	}
	_, err = io.WriteString(w, string(output))
	if err != nil {
		fmt.Println(err.Error())
	}

	// Print the equation, value and result to the server console
	if debug {
		fmt.Printf("Solve equation: '%v'\nResult: '%s'\n", newEq, output)
	}
}

// Validates inputs
func validate(inpEq string, inpMin string, inpMax string, inpStep string) (newEq string, min float64, max float64, step float64, err error) {
	// Validate the equation string
	s, err := validateEqStr(inpEq)
	if err != nil {
		return
	}
	newEq = s

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

// Validates an equation string
func validateEqStr(s string) (t string, err error) {
	var badChars strings.Builder
	var tmp strings.Builder
	for _, j := range s {
		switch j {
		case 'x', '+', '-', '*', '/', '^', ' ', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '(', ')', '[', ']':
			// The character is valid
			tmp.WriteRune(j)
		case 'X':
			// The character is valid
			tmp.WriteRune('x')
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
	t = tmp.String()
	return
}
