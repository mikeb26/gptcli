package ui

import (
	"bufio"
	"fmt"
	"os"

	"github.com/mikeb26/gptcli/internal/types"
)

// StdioUI implements the GptCliUIOptionDialogue and GptCliUIInputDialogue
// interfaces using standard input/output.
type StdioUI struct{}

func NewStdioUI() *StdioUI {
	return &StdioUI{}
}

// SelectOption presents a list of options to the user on stdout and reads
// their selection from stdin. The user is prompted to enter the numeric index
// of the desired option. It returns an error if the input is invalid.
func (s *StdioUI) SelectOption(userPrompt string,
	choices []types.GptCliUIOption) (types.GptCliUIOption, error) {

	if len(choices) == 0 {
		return types.GptCliUIOption{}, fmt.Errorf("no choices provided")
	}

	fmt.Fprintln(os.Stdout, userPrompt)
	for i, c := range choices {
		fmt.Fprintf(os.Stdout, "%d) %s\n", i+1, c.Label)
	}
	fmt.Fprint(os.Stdout, "Enter choice number: ")

	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return types.GptCliUIOption{}, err
		}

		var idx int
		_, err = fmt.Sscanf(line, "%d", &idx)
		if err != nil || idx < 1 || idx > len(choices) {
			fmt.Fprint(os.Stdout,
				"Invalid selection. Please enter a number between 1 and ", len(choices), ": ")
			continue
		}

		return choices[idx-1], nil
	}
}

// Get prompts the user for a line of input and returns it, stripping the
// trailing newline.
func (s *StdioUI) Get(userPrompt string) (string, error) {

	fmt.Fprint(os.Stdout, userPrompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	// Trim trailing CR/LF
	if len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}

	return line, nil
}
