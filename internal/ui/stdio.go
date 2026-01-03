/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/mikeb26/gptcli/internal/types"
)

// StdioUI implements the UI interface using standard input/output.
type StdioUI struct {
	mu     sync.Mutex
	input  *bufio.Reader
	output io.Writer
}

func NewStdioUI() *StdioUI {
	return &StdioUI{
		input:  bufio.NewReader(os.Stdin),
		output: os.Stdout,
	}
}

func (s *StdioUI) WithReader(reader io.Reader) *StdioUI {
	s.input = bufio.NewReader(reader)
	return s
}

func (s *StdioUI) WithBufReader(reader *bufio.Reader) *StdioUI {
	s.input = reader
	return s
}

func (s *StdioUI) WithWriter(writer io.Writer) *StdioUI {
	s.output = writer
	return s
}

// getUnlocked is a helper that performs the core logic of Get without taking
// the mutex. It is intended to be called by methods that already hold s.mu.
func (s *StdioUI) getUnlocked(userPrompt string) (string, error) {
	fmt.Fprint(s.output, userPrompt)
	line, err := s.input.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	line = strings.TrimSpace(line)

	return line, nil
}

// SelectOption presents a list of options to the user on stdout and reads
// their selection from stdin. The user is prompted to enter the numeric index
// of the desired option. It returns an error if the input is invalid.
func (s *StdioUI) SelectOption(userPrompt string,
	choices []types.UIOption) (types.UIOption, error) {

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(choices) == 0 {
		return types.UIOption{}, fmt.Errorf("no choices provided")
	}

	fmt.Fprintln(s.output, userPrompt)
	for i, c := range choices {
		fmt.Fprintf(s.output, "%d) %s\n", i+1, c.Label)
	}
	fmt.Fprint(s.output, "Enter choice number: ")

	for {
		line, err := s.input.ReadString('\n')
		if err != nil {
			return types.UIOption{}, err
		}

		var idx int
		_, err = fmt.Sscanf(line, "%d", &idx)
		if err != nil || idx < 1 || idx > len(choices) {
			fmt.Fprint(s.output,
				"Invalid selection. Please enter a number between 1 and ",
				len(choices), ": ")
			continue
		}

		return choices[idx-1], nil
	}
}

// SelectBool presents a true and false option to the user on stdout and reads
// their selection from stdin. It returns an error if the input is invalid.
func (s *StdioUI) SelectBool(userPrompt string,
	trueOption, falseOption types.UIOption,
	defaultOpt *bool) (bool, error) {

	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		result, err := s.getUnlocked(userPrompt)
		if err != nil {
			return false, err
		}

		if strings.ToLower(result) == strings.ToLower(trueOption.Label) {
			return true, nil
		} else if strings.ToLower(result) == strings.ToLower(falseOption.Label) {
			return false, nil
		} else if result == "" && defaultOpt != nil {
			return *defaultOpt, nil
		} // else

		fmt.Fprint(s.output, "Invalid selection. ")
	}
}

// Get prompts the user for a line of input and returns it, stripping the
// trailing newline.
func (s *StdioUI) Get(userPrompt string) (string, error) {

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getUnlocked(userPrompt)
}
