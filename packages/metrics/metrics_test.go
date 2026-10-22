/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckListen(t *testing.T) {
	for listen, ok := range map[string]bool{
		"":               true,
		"127.0.0.1:7090": true,
		"localhost:7090": true,
		"[::1]:7090":     true,
		"0.0.0.0:7090":   false,
		"10.0.0.5:7090":  false,
		"example:7090":   false,
		"127.0.0.1":      false,
	} {
		if err := CheckListen(listen); (err == nil) != ok {
			t.Errorf("CheckListen(%q) = %v, want ok %v", listen, err, ok)
		}
	}
}

func TestHandlerServesTheMetricsAndTheProfiler(t *testing.T) {
	BlockStage.WithLabelValues(KindApply, StageExecute).Observe(0.01)
	DBStatements.WithLabelValues("query").Inc()
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	get := func(path string) string {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	body := get("/metrics")
	for _, name := range []string{
		`ibax_block_stage_seconds_count{kind="apply",stage="execute"} 1`,
		`ibax_db_statements_total{op="query"} 1`,
		"go_goroutines",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("/metrics has no %s", name)
		}
	}
	if !strings.Contains(get("/debug/pprof/"), "goroutine") {
		t.Error("/debug/pprof/ lists no goroutine profile")
	}
}
