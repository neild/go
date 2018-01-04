// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Serving of pprof-like profiles.

package main

import (
	"bufio"
	"fmt"
	"internal/trace"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/google/pprof/profile"
)

func goCmd() string {
	var exeSuffix string
	if runtime.GOOS == "windows" {
		exeSuffix = ".exe"
	}
	path := filepath.Join(runtime.GOROOT(), "bin", "go"+exeSuffix)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return "go"
}

func init() {
	http.HandleFunc("/io", serveSVGProfile(pprofIO))
	http.HandleFunc("/block", serveSVGProfile(pprofBlock))
	http.HandleFunc("/syscall", serveSVGProfile(pprofSyscall))
	http.HandleFunc("/sched", serveSVGProfile(pprofSched))
}

// Record represents one entry in pprof-like profiles.
type Record struct {
	stk  []*trace.Frame
	n    uint64
	time int64
}

// pprofMatchingGoroutines parses the goroutine type id string (i.e. pc)
// and returns the ids of goroutines of the matching type.
// If the id string is empty, returns nil without an error.
func pprofMatchingGoroutines(id string, events []*trace.Event) (map[uint64]bool, error) {
	if id == "" {
		return nil, nil
	}
	pc, err := strconv.ParseUint(id, 10, 64) // id is string
	if err != nil {
		return nil, fmt.Errorf("invalid goroutine type: %v", id)
	}
	analyzeGoroutines(events)
	var res map[uint64]bool
	for _, g := range gs {
		if g.PC != pc {
			continue
		}
		if res == nil {
			res = make(map[uint64]bool)
		}
		res[g.ID] = true
	}
	if len(res) == 0 && id != "" {
		return nil, fmt.Errorf("failed to find matching goroutines for id: %s", id)
	}
	return res, nil
}

// pprofIO generates IO pprof-like profile (time spent in IO wait,
// currently only network blocking event).
func pprofIO(w io.Writer, id string) error {
	events, err := parseEvents()
	if err != nil {
		return err
	}
	goroutines, err := pprofMatchingGoroutines(id, events)
	if err != nil {
		return err
	}

	prof := make(map[uint64]Record)
	for _, ev := range events {
		if ev.Type != trace.EvGoBlockNet || ev.Link == nil || ev.StkID == 0 || len(ev.Stk) == 0 {
			continue
		}
		if goroutines != nil && !goroutines[ev.G] {
			continue
		}
		rec := prof[ev.StkID]
		rec.stk = ev.Stk
		rec.n++
		rec.time += ev.Link.Ts - ev.Ts
		prof[ev.StkID] = rec
	}
	return buildProfile(prof).Write(w)
}

// pprofBlock generates blocking pprof-like profile (time spent blocked on synchronization primitives).
func pprofBlock(w io.Writer, id string) error {
	events, err := parseEvents()
	if err != nil {
		return err
	}
	goroutines, err := pprofMatchingGoroutines(id, events)
	if err != nil {
		return err
	}

	prof := make(map[uint64]Record)
	for _, ev := range events {
		switch ev.Type {
		case trace.EvGoBlockSend, trace.EvGoBlockRecv, trace.EvGoBlockSelect,
			trace.EvGoBlockSync, trace.EvGoBlockCond, trace.EvGoBlockGC:
			// TODO(hyangah): figure out why EvGoBlockGC should be here.
			// EvGoBlockGC indicates the goroutine blocks on GC assist, not
			// on synchronization primitives.
		default:
			continue
		}
		if ev.Link == nil || ev.StkID == 0 || len(ev.Stk) == 0 {
			continue
		}
		if goroutines != nil && !goroutines[ev.G] {
			continue
		}
		rec := prof[ev.StkID]
		rec.stk = ev.Stk
		rec.n++
		rec.time += ev.Link.Ts - ev.Ts
		prof[ev.StkID] = rec
	}
	return buildProfile(prof).Write(w)
}

// pprofSyscall generates syscall pprof-like profile (time spent blocked in syscalls).
func pprofSyscall(w io.Writer, id string) error {

	events, err := parseEvents()
	if err != nil {
		return err
	}
	goroutines, err := pprofMatchingGoroutines(id, events)
	if err != nil {
		return err
	}

	prof := make(map[uint64]Record)
	for _, ev := range events {
		if ev.Type != trace.EvGoSysCall || ev.Link == nil || ev.StkID == 0 || len(ev.Stk) == 0 {
			continue
		}
		if goroutines != nil && !goroutines[ev.G] {
			continue
		}
		rec := prof[ev.StkID]
		rec.stk = ev.Stk
		rec.n++
		rec.time += ev.Link.Ts - ev.Ts
		prof[ev.StkID] = rec
	}
	return buildProfile(prof).Write(w)
}

// pprofSched generates scheduler latency pprof-like profile
// (time between a goroutine become runnable and actually scheduled for execution).
func pprofSched(w io.Writer, id string) error {
	events, err := parseEvents()
	if err != nil {
		return err
	}
	goroutines, err := pprofMatchingGoroutines(id, events)
	if err != nil {
		return err
	}

	prof := make(map[uint64]Record)
	for _, ev := range events {
		if (ev.Type != trace.EvGoUnblock && ev.Type != trace.EvGoCreate) ||
			ev.Link == nil || ev.StkID == 0 || len(ev.Stk) == 0 {
			continue
		}
		if goroutines != nil && !goroutines[ev.G] {
			continue
		}
		rec := prof[ev.StkID]
		rec.stk = ev.Stk
		rec.n++
		rec.time += ev.Link.Ts - ev.Ts
		prof[ev.StkID] = rec
	}
	return buildProfile(prof).Write(w)
}

// serveSVGProfile serves pprof-like profile generated by prof as svg.
func serveSVGProfile(prof func(w io.Writer, id string) error) http.HandlerFunc {
	return func w, r {

		if r.FormValue("raw") != "" {
			w.Header().Set("Content-Type", "application/octet-stream")
			if err := prof(w, r.FormValue("id")); err != nil {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("X-Go-Pprof", "1")
				http.Error(w, fmt.Sprintf("failed to get profile: %v", err), http.StatusInternalServerError)
				return
			}
			return
		}

		blockf, err := ioutil.TempFile("", "block")
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create temp file: %v", err), http.StatusInternalServerError)
			return
		}
		defer func {
			blockf.Close()
			os.Remove(blockf.Name())
		}()
		blockb := bufio.NewWriter(blockf)
		if err := prof(blockb, r.FormValue("id")); err != nil {
			http.Error(w, fmt.Sprintf("failed to generate profile: %v", err), http.StatusInternalServerError)
			return
		}
		if err := blockb.Flush(); err != nil {
			http.Error(w, fmt.Sprintf("failed to flush temp file: %v", err), http.StatusInternalServerError)
			return
		}
		if err := blockf.Close(); err != nil {
			http.Error(w, fmt.Sprintf("failed to close temp file: %v", err), http.StatusInternalServerError)
			return
		}
		svgFilename := blockf.Name() + ".svg"
		if output, err := exec.Command(goCmd(), "tool", "pprof", "-svg", "-output", svgFilename, blockf.Name()).CombinedOutput(); err != nil {
			http.Error(w, fmt.Sprintf("failed to execute go tool pprof: %v\n%s", err, output), http.StatusInternalServerError)
			return
		}
		defer os.Remove(svgFilename)
		w.Header().Set("Content-Type", "image/svg+xml")
		http.ServeFile(w, r, svgFilename)
	}
}

func buildProfile(prof map[uint64]Record) *profile.Profile {
	p := &profile.Profile{
		PeriodType: &profile.ValueType{Type: "trace", Unit: "count"},
		Period:     1,
		SampleType: []*profile.ValueType{
			{Type: "contentions", Unit: "count"},
			{Type: "delay", Unit: "nanoseconds"},
		},
	}
	locs := make(map[uint64]*profile.Location)
	funcs := make(map[string]*profile.Function)
	for _, rec := range prof {
		var sloc []*profile.Location
		for _, frame := range rec.stk {
			loc := locs[frame.PC]
			if loc == nil {
				fn := funcs[frame.File+frame.Fn]
				if fn == nil {
					fn = &profile.Function{
						ID:         uint64(len(p.Function) + 1),
						Name:       frame.Fn,
						SystemName: frame.Fn,
						Filename:   frame.File,
					}
					p.Function = append(p.Function, fn)
					funcs[frame.File+frame.Fn] = fn
				}
				loc = &profile.Location{
					ID:      uint64(len(p.Location) + 1),
					Address: frame.PC,
					Line: []profile.Line{
						profile.Line{
							Function: fn,
							Line:     int64(frame.Line),
						},
					},
				}
				p.Location = append(p.Location, loc)
				locs[frame.PC] = loc
			}
			sloc = append(sloc, loc)
		}
		p.Sample = append(p.Sample, &profile.Sample{
			Value:    []int64{int64(rec.n), rec.time},
			Location: sloc,
		})
	}
	return p
}
