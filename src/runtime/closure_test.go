// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package runtime_test

import "testing"

var s int

func BenchmarkCallClosure(b *testing.B) {
	for i := 0; i < b.N; i++ {
		s += func ii { return 2 * ii }(i)
	}
}

func BenchmarkCallClosure1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		j := i
		s += func ii { return 2*ii + j }(i)
	}
}

var ss *int

func BenchmarkCallClosure2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		j := i
		s += func {
			ss = &j
			return 2
		}()
	}
}

func addr1(x int) *int {
	return func { return &x }()
}

func BenchmarkCallClosure3(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ss = addr1(i)
	}
}

func addr2() (x int, p *int) {
	return 0, func { return &x }()
}

func BenchmarkCallClosure4(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, ss = addr2()
	}
}
