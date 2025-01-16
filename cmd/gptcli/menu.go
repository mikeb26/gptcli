/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	gc "github.com/rthornton128/goncurses"
)

func showMenu(gptCliCtx *GptCliContext, menuText string) error {
	scr, err := gc.Init()
	if err != nil {
		return fmt.Errorf("Failed to initialize screen: %w", err)
	}

	gc.CBreak(true)
	gc.Echo(false)
	gc.Cursor(0)
	gc.SetEscDelay(20)
	scr.Keypad(true)
	scr.Timeout(20)

	selectedThread := 1
	data := strings.Split(menuText, "\n")

	for {
		scr.Clear()

		scr.Printf("%s", threadGroupHeaderString())
		for ii, row := range data {
			if ii+1 == selectedThread {
				scr.AttrOn(gc.A_REVERSE)
			} else {
				scr.AttrOff(gc.A_REVERSE)
			}
			scr.Printf("%s\n", row)
		}
		scr.AttrOff(gc.A_REVERSE)

		ch := scr.GetChar()
		switch ch {
		case 0:
			continue
		case gc.KEY_ESC:
			fallthrough
		case 'd' - 'a' + 1: // control-d
			fallthrough
		case 'q':
			gc.End()
			return nil
		case gc.KEY_UP:
			if selectedThread > 1 {
				selectedThread--
			}
		case gc.KEY_DOWN:
			if selectedThread < len(data)-1 {
				selectedThread++
			}
		case gc.KEY_RETURN:
			gc.End()
			gptCliCtx.curThreadGroup = gptCliCtx.mainThreadGroup
			return gptCliCtx.mainThreadGroup.threadSwitch(selectedThread)
		}
	}

	return fmt.Errorf("BUG: unreachable")
}

func menuMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.mainThreadGroup.totThreads == 0 {
		fmt.Printf("%v.\n", ErrNoThreadsExist)
		return nil
	}

	//showAll := false

	f := flag.NewFlagSet("ls", flag.ContinueOnError)
	//	f.BoolVar(&showAll, "all", false, "Also show archive threads")
	err := f.Parse(args[1:])
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString(gptCliCtx.mainThreadGroup.String(false, false))
	//	if showAll {
	//		sb.WriteString(gptCliCtx.archiveThreadGroup.String(false, false))
	//	}

	return showMenu(gptCliCtx, sb.String())
}
