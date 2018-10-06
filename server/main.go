package main

// A simple server to return derivatives and basic solved equations to the wasmGraph5 front end

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	eq "github.com/corywalker/expreduce/expreduce"
)

var (
	debug = true // If true, some debugging info is printed to the javascript console
)

func main() {
	http.HandleFunc("/derivstr/", derivStrHandler)
	http.HandleFunc("/solvederiv/", solveDerivHandler)
	http.HandleFunc("/solveeq/", solveEqHandler)
	err := http.ListenAndServe("0.0.0.0:8080", nil)
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
	newEq, err := validate(inp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		// Display message on server console
		if debug {
			fmt.Println(err.Error())
		}
		return
	}

	// Return the derivative of the equation
	str := fmt.Sprintf("D[%s, x]", newEq.String())
	state := eq.NewEvalState()
	expr := eq.Interp(str, state)
	result := expr.Eval(state)
	result = state.ProcessTopLevelResult(expr, result)
	deriv := result.StringForm(eq.ActualStringFormArgsFull("OutputForm", state))
	io.WriteString(w, deriv)

	// Print the equation and derivative to the server console
	if debug {
		fmt.Printf("Equation: '%v' - Derivative: '%v'\n", newEq.String(), deriv)
	}
}

// Returns the derivative for a given input equation string + value
//   * Use a browser to simulate a call with (eg): http://0.0.0.0:8080/solvederiv/?eq=x^2&val=2.0
func solveDerivHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential equation string and value
	inpEq := r.FormValue("eq")
	inpVal := r.FormValue("val")
	if debug {
		fmt.Printf("Derivative input: equation '%v' with value '%v' received\n", inpEq, inpVal)
	}

	// Validate equation string
	newEq, err := validate(inpEq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		if debug {
			fmt.Println(err.Error())
		}
		return
	}

	// Validate floating point value string
	val, err := strconv.ParseFloat(inpVal, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		if debug {
			fmt.Println(err.Error())
		}
		return
	}

	// Solve the derivative equation
	str := fmt.Sprintf("D[%s,x] /. x -> %.2f", newEq.String(), val)
	state := eq.NewEvalState()
	expr := eq.Interp(str, state)
	result := expr.Eval(state)
	result = state.ProcessTopLevelResult(expr, result)
	solve := result.StringForm(eq.ActualStringFormArgsFull("OutputForm", state))
	io.WriteString(w, solve)

	// Print the equation, value and result to the server console
	if debug {
		fmt.Printf("Derivative equation: '%v', value: '%v' - Result: '%v'\n", newEq.String(), val, solve)
	}
}

// Returns the solved equation for a given input equation string + value
//   * Use a browser to simulate a call with (eg): http://0.0.0.0:8080/solveeq/?eq=x^2&val=2.0
func solveEqHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential equation string and value
	inpEq := r.FormValue("eq")
	inpVal := r.FormValue("val")
	if debug {
		fmt.Printf("Solve input: equation '%v' with value '%v' received\n", inpEq, inpVal)
	}

	// Validate equation string
	newEq, err := validate(inpEq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		if debug {
			fmt.Println(err.Error())
		}
		return
	}

	// Validate floating point value string
	val, err := strconv.ParseFloat(inpVal, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		if debug {
			fmt.Println(err.Error())
		}
		return
	}

	// Solve the equation
	str := fmt.Sprintf("f[x_] := %s", newEq.String())
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
	if debug {
		fmt.Printf("Equation: '%v', value: '%v' - Result: '%v'\n", newEq.String(), val, result)
	}
}

// Validates an input string
func validate(s string) (newEq strings.Builder, err error) {
	var badChars strings.Builder
	for _, j := range s {
		switch j {
		case 'x', '+', '-', '*', '/', '^', ' ', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '(', ')', '[', ']':
			// The character is valid
			newEq.WriteRune(j)
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
	return
}
