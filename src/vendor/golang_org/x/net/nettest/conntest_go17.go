// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.7

package nettest

import "testing"

func testConn(t *testing.T, mp MakePipe) {
	// Use subtests on Go 1.7 and above since it is better organized.
	t.Run("BasicIO", func t { timeoutWrapper(t, mp, testBasicIO) })
	t.Run("PingPong", func t { timeoutWrapper(t, mp, testPingPong) })
	t.Run("RacyRead", func t { timeoutWrapper(t, mp, testRacyRead) })
	t.Run("RacyWrite", func t { timeoutWrapper(t, mp, testRacyWrite) })
	t.Run("ReadTimeout", func t { timeoutWrapper(t, mp, testReadTimeout) })
	t.Run("WriteTimeout", func t { timeoutWrapper(t, mp, testWriteTimeout) })
	t.Run("PastTimeout", func t { timeoutWrapper(t, mp, testPastTimeout) })
	t.Run("PresentTimeout", func t { timeoutWrapper(t, mp, testPresentTimeout) })
	t.Run("FutureTimeout", func t { timeoutWrapper(t, mp, testFutureTimeout) })
	t.Run("CloseTimeout", func t { timeoutWrapper(t, mp, testCloseTimeout) })
	t.Run("ConcurrentMethods", func t { timeoutWrapper(t, mp, testConcurrentMethods) })
}
