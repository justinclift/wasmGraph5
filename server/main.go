package main

// A simple server to return derivatives and basic solved equations to the wasmGraph5 front end
//   * Use a browser to simulate a call with (eg): http://0.0.0.0:8080/deriv/?eq=x^2

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	eq "github.com/corywalker/expreduce/expreduce"
)

var (
	debug               = true // If true, some debugging info is printed to the javascript console
)

func main() {
	http.HandleFunc("/deriv/", derivHandler)
	err := http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func derivHandler (w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential equation string
	inp := r.FormValue("eq")
	if debug {
		fmt.Printf("String '%v' received\n", inp)
	}

	// Input validation
	var badChars, newEq strings.Builder
	for _, j := range inp {
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
		resp := fmt.Sprintf("Bad character(s) in equation input string: %s\n", badChars.String())
		http.Error(w, resp, http.StatusInternalServerError)

		// Display message on server console
		if debug {
			fmt.Println(resp)
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
