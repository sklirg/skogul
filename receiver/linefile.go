/*
 * skogul, linefile receiver
 *
 * Copyright (c) 2019 Telenor Norge AS
 * Author(s):
 *  - Kristian Lyngstøl <kly@kly.no>
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation; either
 * version 2.1 of the License, or (at your option) any later version.
 *
 * This library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA
 * 02110-1301  USA
 */

package receiver

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/telenornms/skogul"
)

var lfLog = skogul.Logger("receiver", "linefile")

// LineFile will keep reading File over and over again, assuming one
// collection per line. Best suited for pointing at a FIFO, which will
// allow you to 'cat' stuff to Skogul.
type LineFile struct {
	File    string            `doc:"Path to the fifo or file from which to read from repeatedly."`
	Handler skogul.HandlerRef `doc:"Handler used to parse and transform and send data."`
	Delay   skogul.Duration   `doc:"Delay before re-opening the file, if any."`
}

// Common routine for both fifo and stdin
func (lf *LineFile) read() error {
	f, err := os.Open(lf.File)
	if err != nil {
		return fmt.Errorf("unable to open file %s: %w", lf.File, err)
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		bytes := scanner.Bytes()
		if err := lf.Handler.H.Handle(bytes); err != nil {
			lfLog.WithError(err).Error("Failed to send metric")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("unable to scan file: %w", err)
	}
	return nil
}

// Start never returns.
func (lf *LineFile) Start() error {
	for {
		if err := lf.read(); err != nil {
			lfLog.WithError(err).Error("Unable to read file")
		}
		if lf.Delay.Duration != 0 {
			time.Sleep(lf.Delay.Duration)
		}
	}
}

// File reads from a FILE, a single JSON object per line, and
// exits at EOF.
type File struct {
	File    string            `doc:"Path to the file to read from once."`
	Handler skogul.HandlerRef `doc:"Handler used to parse, transform and send data."`
	lf      LineFile
}

// Start reads a file once, then returns.
func (s *File) Start() error {
	s.lf.File = s.File
	s.lf.Handler = s.Handler
	return s.lf.read()
}

// Stdin reads from /dev/stdin
type Stdin struct {
	Handler skogul.HandlerRef `doc:"Handler used to parse, transform and send data."`
	lf      LineFile
}

// Start reads from stdin until EOF, then returns
func (s *Stdin) Start() error {
	s.lf.File = "/dev/stdin"
	s.lf.Handler = s.Handler
	return s.lf.read()
}

// WholeFile reads the whole file and parses it as a single container
type WholeFile struct {
	File      string            `doc:"Path to the file to read from."`
	Handler   skogul.HandlerRef `doc:"Handler used to parse, transform and send data."`
	Frequency skogul.Duration   `doc:"How often to re-read the same file. Leave blank or set to a negative value to only read once."`
}

func (wf *WholeFile) read() error {
	b, err := os.ReadFile(wf.File)
	if err != nil {
		return fmt.Errorf("unable to open file from %s: %w", wf.File, err)
	}
	err = wf.Handler.H.Handle(b)
	if err != nil {
		return fmt.Errorf("unable to handle content: %w", err)
	}
	return nil
}

// Start never returns
func (wf *WholeFile) Start() error {
	freq := wf.Frequency.Duration
	sleep := freq >= time.Nanosecond
	for {
		err := wf.read()
		if err != nil {
			lfLog.WithError(err).Errorf("whole file reader %s", skogul.Identity[wf])
		}
		if sleep {
			time.Sleep(freq)
		} else {
			for {
				time.Sleep(time.Hour)
			}
		}
	}
}

type LineFileAdvanced struct {
	File    string            `doc:"Path to the fifo or file from which to read from repeatedly."`
	NewFile string            `doc:"Path to the fifo or file that 'File'will be moved to."`
	Handler skogul.HandlerRef `doc:"Handler used to parse and transform and send data."`
	Delay   skogul.Duration   `doc:"Delay before re-opening the file, if any."`
	Post    string            `doc:"Shell command to execute after reading file is finished."`
	Shell   string            `doc:"Shell used to execute post command."`
}

func (lf *LineFileAdvanced) read() error {
	lf.moveFile(lf.File, lf.NewFile)

	f, err := os.Open(lf.NewFile)
	if err != nil {
		return fmt.Errorf("unable to open file %s: %w", lf.NewFile, err)
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		bytes := scanner.Bytes()
		if err := lf.Handler.H.Handle(bytes); err != nil {
			lfLog.WithError(err).Error("Failed to send metric")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("unable to scan file: %w", err)
	}

	if lf.Post != "" {
		shell := strings.Split(lf.Shell, " ")
		c := exec.Command(shell[0], "-c", lf.Post)
		_, err := c.Output()

		if err != nil {
			lfLog.WithError(err).Errorf("executing post command %s failed", lf.Post)
			return err
		}
	}

	return nil
}

// Start never returns.
func (lf *LineFileAdvanced) Start() error {
	if lf.Shell == "" {
		lf.Shell = "/bin/sh"
	}

	for {
		if err := lf.read(); err != nil {
			lfLog.WithError(err).Error("Unable to read file")
		}
		if lf.Delay.Duration != 0 {
			time.Sleep(lf.Delay.Duration)
		}
	}
}

func (lf *LineFileAdvanced) moveFile(oldPath string, newPath string) {
	err := os.Rename(oldPath, newPath)
	if err != nil {
		lfLog.WithError(err).Errorf("could not move file from %s to %s", oldPath, newPath)
		return
	}
}
