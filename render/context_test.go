package render

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/reidransom/liquid/parser"
)

func addContextTestTags(s Config) {
	s.AddTag("test_bindings", func(string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			b := c.Bindings()
			_, err := fmt.Fprintf(w, "%v", b["x"])
			return err
		}, nil
	})
	s.AddTag("test_get", func(string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			v := c.Get("x")
			_, err := fmt.Fprintf(w, "%v", v)
			return err
		}, nil
	})
	s.AddTag("test_set", func(string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			c.Set("x", 999)
			_, err := fmt.Fprintf(w, "%v", c.Get("x"))
			return err
		}, nil
	})
	s.AddBlock("test_inner_string").Compiler(func(bn BlockNode) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			s, err := c.InnerString()
			if err != nil {
				return err
			}
			_, err = io.WriteString(w, "inner:"+s)
			return err
		}, nil
	})
	s.AddBlock("test_render_children").Compiler(func(bn BlockNode) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			_, _ = io.WriteString(w, "before:")
			rerr := c.RenderChildren(w)
			if rerr != nil {
				return rerr
			}
			_, err := io.WriteString(w, ":after")
			return err
		}, nil
	})
	s.AddTag("test_set_path", func(string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			err := c.SetPath([]string{"page", "url"}, "/about/")
			if err != nil {
				return err
			}
			v := c.Get("page")
			m, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("page is not a map")
			}
			_, err = fmt.Fprintf(w, "%v", m["url"])
			return err
		}, nil
	})
	s.AddTag("test_evaluate_string", func(string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			v, err := c.EvaluateString(c.TagArgs())
			if err != nil {
				return err
			}

			_, err = fmt.Fprint(w, v)

			return err
		}, nil
	})
	s.AddBlock("parse").Compiler(func(c BlockNode) (func(io.Writer, Context) error, error) {
		a := c.Args

		return func(w io.Writer, c Context) error {
			_, err := io.WriteString(w, a)
			return err
		}, nil
	})
	s.AddTag("test_tag_name", func(string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			_, err := io.WriteString(w, c.TagName())
			return err
		}, nil
	})
	s.AddTag("test_expand_tag_arg", func(string) (func(w io.Writer, c Context) error, error) {
		return func(w io.Writer, c Context) error {
			s, err := c.ExpandTagArg()
			if err != nil {
				return err
			}

			_, err = io.WriteString(w, s)

			return err
		}, nil
	})
	s.AddTag("test_render_file", func(filename string) (func(w io.Writer, c Context) error, error) {
		return func(w io.Writer, c Context) error {
			s, err := c.RenderFile(filename, map[string]any{"shadowed": 2})
			if err != nil {
				return err
			}

			_, err = io.WriteString(w, s)

			return err
		}, nil
	})
	s.AddBlock("test_block_sourcefile").Compiler(func(c BlockNode) (func(w io.Writer, c Context) error, error) {
		return func(w io.Writer, c Context) error {
			_, err := io.WriteString(w, c.SourceFile())
			return err
		}, nil
	})
	s.AddBlock("test_block_wraperror").Compiler(func(c BlockNode) (func(w io.Writer, c Context) error, error) {
		return func(w io.Writer, c Context) error {
			return c.WrapError(errors.New("giftwrapped"))
		}, nil
	})
	s.AddBlock("test_block_errorf").Compiler(func(c BlockNode) (func(w io.Writer, c Context) error, error) {
		return func(w io.Writer, c Context) error {
			return c.Errorf("giftwrapped")
		}, nil
	})
}

type countingTemplateStore struct {
	templates map[string][]byte
	errs      map[string]error
	reads     atomic.Int32
}

func (s *countingTemplateStore) ReadTemplate(filename string) ([]byte, error) {
	s.reads.Add(1)
	if err, ok := s.errs[filename]; ok {
		return nil, err
	}
	if source, ok := s.templates[filename]; ok {
		return source, nil
	}
	return nil, fs.ErrNotExist
}

func addFileCacheTestTag(cfg *Config) {
	cfg.AddTag("render_file", func(filename string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			s, err := c.RenderFile(filename, map[string]any{"include": c.Get("value")})
			if err != nil {
				return err
			}
			_, err = io.WriteString(w, s)
			return err
		}, nil
	})
}

func addFileCacheCompileCounter(cfg *Config, count *atomic.Int32) {
	cfg.AddTag("file_cache_compile", func(string) (func(io.Writer, Context) error, error) {
		count.Add(1)
		return func(io.Writer, Context) error {
			return nil
		}, nil
	})
}

type blockingTemplateStore struct {
	started chan string
	release map[string]chan struct{}
}

func (s *blockingTemplateStore) ReadTemplate(filename string) ([]byte, error) {
	s.started <- filename
	<-s.release[filename]
	return []byte("partial"), nil
}

var contextTests = []struct{ in, out string }{
	{`{% parse args %}{% endparse %}`, "args"},
	{`{% test_evaluate_string x %}`, "123"},
	{`{% test_expand_tag_arg x %}`, "x"},
	{`{% test_expand_tag_arg {{x}} %}`, "123"},
	{`{% test_tag_name %}`, "test_tag_name"},
	{
		`{% test_render_file testdata/render_file.txt %}; x={{ x }}; unshadowed={{ shadowed }}`,
		"999rendered shadowed=2; x=999; unshadowed=1",
	},
	{`{% test_block_sourcefile %}x{% endtest_block_sourcefile %}`, ``},
	{`{% test_bindings %}`, "123"},
	{`{% test_get %}`, "123"},
	{`{% test_set %}`, "999"},
	{`{% test_inner_string %}hello world{% endtest_inner_string %}`, "inner:hello world"},
	{`{% test_render_children %}content{% endtest_render_children %}`, "before:content:after"},
	{`{% test_set_path %}`, "/about/"},
}

var contextErrorTests = []struct{ in, expect string }{
	{`{% test_evaluate_string syntax error %}`, "syntax error"},
	{`{% test_expand_tag_arg {{ syntax error }} %}`, "syntax error"},
	{`{% test_expand_tag_arg {{ x | undefined_filter }} %}`, "undefined filter"},
	{`{% test_render_file testdata/render_file_syntax_error.txt %}`, "syntax error"},
	{`{% test_render_file testdata/render_file_runtime_error.txt %}`, "undefined tag"},
	{`{% test_block_wraperror %}{% endtest_block_wraperror %}`, "giftwrapped"},
	{`{% test_block_errorf %}{% endtest_block_errorf %}`, "giftwrapped"},
}

var contextTestBindings = map[string]any{
	"x":        123,
	"shadowed": 1,
}

func TestContext(t *testing.T) {
	cfg := NewConfig()
	addContextTestTags(cfg)

	for i, test := range contextTests {
		t.Run(fmt.Sprintf("%02d", i+1), func(t *testing.T) {
			root, err := cfg.Compile(test.in, parser.SourceLoc{})
			require.NoErrorf(t, err, test.in)

			buf := new(bytes.Buffer)
			err = Render(root, buf, contextTestBindings, cfg)
			require.NoErrorf(t, err, test.in)
			require.Equalf(t, test.out, buf.String(), test.in)
		})
	}
}

func TestContext_errors(t *testing.T) {
	cfg := NewConfig()
	addContextTestTags(cfg)

	for i, test := range contextErrorTests {
		t.Run(fmt.Sprintf("%02d", i+1), func(t *testing.T) {
			root, err := cfg.Compile(test.in, parser.SourceLoc{})
			require.NoErrorf(t, err, test.in)
			err = Render(root, io.Discard, contextTestBindings, cfg)
			require.Errorf(t, err, test.in)
			require.Containsf(t, err.Error(), test.expect, test.in)
		})
	}
}

func TestSetPath(t *testing.T) {
	cfg := NewConfig()
	addContextTestTags(cfg)

	t.Run("single path", func(t *testing.T) {
		cfg.AddTag("sp_single", func(string) (func(io.Writer, Context) error, error) {
			return func(w io.Writer, c Context) error {
				err := c.SetPath([]string{"newvar"}, 42)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(w, "%v", c.Get("newvar"))
				return err
			}, nil
		})
		root, err := cfg.Compile(`{% sp_single %}`, parser.SourceLoc{})
		require.NoError(t, err)
		buf := new(bytes.Buffer)
		err = Render(root, buf, map[string]any{}, cfg)
		require.NoError(t, err)
		require.Equal(t, "42", buf.String())
	})

	t.Run("intermediate creation", func(t *testing.T) {
		cfg.AddTag("sp_create", func(string) (func(io.Writer, Context) error, error) {
			return func(w io.Writer, c Context) error {
				err := c.SetPath([]string{"a", "b", "c"}, "deep")
				if err != nil {
					return err
				}
				a := c.Get("a")
				m1 := a.(map[string]any)
				m2 := m1["b"].(map[string]any)
				_, err = fmt.Fprintf(w, "%v", m2["c"])
				return err
			}, nil
		})
		root, err := cfg.Compile(`{% sp_create %}`, parser.SourceLoc{})
		require.NoError(t, err)
		buf := new(bytes.Buffer)
		err = Render(root, buf, map[string]any{}, cfg)
		require.NoError(t, err)
		require.Equal(t, "deep", buf.String())
	})

	t.Run("error on non-map", func(t *testing.T) {
		cfg.AddTag("sp_nonmap", func(string) (func(io.Writer, Context) error, error) {
			return func(w io.Writer, c Context) error {
				return c.SetPath([]string{"x", "sub"}, "val")
			}, nil
		})
		root, err := cfg.Compile(`{% sp_nonmap %}`, parser.SourceLoc{})
		require.NoError(t, err)
		// x=123 (int), so SetPath should fail
		err = Render(root, io.Discard, map[string]any{"x": 123}, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot set property on non-object")
	})

	t.Run("empty path", func(t *testing.T) {
		cfg.AddTag("sp_empty", func(string) (func(io.Writer, Context) error, error) {
			return func(w io.Writer, c Context) error {
				return c.SetPath([]string{}, "val")
			}, nil
		})
		root, err := cfg.Compile(`{% sp_empty %}`, parser.SourceLoc{})
		require.NoError(t, err)
		err = Render(root, io.Discard, map[string]any{}, cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty path")
	})
}

func TestContext_file_not_found_error(t *testing.T) {
	// Test the cause instead of looking for a string, since the error message is
	// different between Darwin and Linux ("no such file") and Windows ("The
	// system cannot find the file specified"), at least.
	//
	// Also see TestIncludeTag_file_not_found_error.
	cfg := NewConfig()
	addContextTestTags(cfg)
	root, err := cfg.Compile(`{% test_render_file testdata/missing_file %}`, parser.SourceLoc{})
	require.NoError(t, err)
	err = Render(root, io.Discard, contextTestBindings, cfg)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err.Cause()))
}

func TestRenderFileCacheIsOptIn(t *testing.T) {
	for _, test := range []struct {
		name          string
		enableCache   bool
		expectedReads int32
		expectedCompiles int32
	}{
		{name: "default", expectedReads: 2, expectedCompiles: 2},
		{name: "enabled", enableCache: true, expectedReads: 1, expectedCompiles: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := NewConfig()
			store := &countingTemplateStore{
				templates: map[string][]byte{
					"partial": []byte(`{% file_cache_compile %}{% assign x = include %}{{ page }}:{{ include }}`),
				},
			}
			var compiles atomic.Int32
			cfg.TemplateStore = store
			addFileCacheTestTag(&cfg)
			addFileCacheCompileCounter(&cfg, &compiles)
			if test.enableCache {
				cfg.EnableFileCache()
			}

			root, err := cfg.Compile(`{% render_file partial %}:{{ include }}:{{ x }}`, parser.SourceLoc{
				Pathname: "layout.liquid",
				LineNo:   1,
			})
			require.NoError(t, err)

			for _, bindings := range []map[string]any{
				{"page": "first", "include": "outer-1", "value": "inner-1"},
				{"page": "second", "include": "outer-2", "value": "inner-2"},
			} {
				buf := new(bytes.Buffer)
				err = Render(root, buf, bindings, cfg)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf("%s:%s:%s:%s", bindings["page"], bindings["value"], bindings["include"], bindings["value"]), buf.String())
			}

			require.Equal(t, test.expectedReads, store.reads.Load())
			require.Equal(t, test.expectedCompiles, compiles.Load())
		})
	}
}

func TestRenderFileCacheCachesNotFound(t *testing.T) {
	cfg := NewConfig()
	store := &countingTemplateStore{}
	cfg.TemplateStore = store
	addFileCacheTestTag(&cfg)
	cfg.EnableFileCache()

	root, err := cfg.Compile(`{% render_file missing %}`, parser.SourceLoc{Pathname: "layout.liquid", LineNo: 1})
	require.NoError(t, err)

	for range 2 {
		err = Render(root, io.Discard, map[string]any{}, cfg)
		require.Error(t, err)
		require.True(t, errors.Is(err.Cause(), fs.ErrNotExist))
	}
	require.Equal(t, int32(1), store.reads.Load())
}

func TestRenderFileCacheKeysCallerLocation(t *testing.T) {
	cfg := NewConfig()
	store := &countingTemplateStore{
		templates: map[string][]byte{"partial": []byte(`{% file_cache_compile %}partial`)},
	}
	var compiles atomic.Int32
	cfg.TemplateStore = store
	addFileCacheTestTag(&cfg)
	addFileCacheCompileCounter(&cfg, &compiles)
	cfg.EnableFileCache()

	rootA, err := cfg.Compile(`{% render_file partial %}`, parser.SourceLoc{Pathname: "first.liquid", LineNo: 10})
	require.NoError(t, err)
	rootB, err := cfg.Compile(`{% render_file partial %}`, parser.SourceLoc{Pathname: "first.liquid", LineNo: 20})
	require.NoError(t, err)
	rootC, err := cfg.Compile(`{% render_file partial %}`, parser.SourceLoc{Pathname: "second.liquid", LineNo: 10})
	require.NoError(t, err)

	for _, root := range []Node{rootA, rootB, rootC, rootA, rootB, rootC} {
		err = Render(root, io.Discard, map[string]any{}, cfg)
		require.NoError(t, err)
	}
	require.Equal(t, int32(3), store.reads.Load())
	require.Equal(t, int32(3), compiles.Load())
}

func TestRenderFileCachePreservesCallerDiagnostics(t *testing.T) {
	cfg := NewConfig()
	cfg.TemplateStore = &countingTemplateStore{
		templates: map[string][]byte{"partial": []byte(`{% caller_error %}`)},
	}
	addFileCacheTestTag(&cfg)
	cfg.AddTag("caller_error", func(string) (func(io.Writer, Context) error, error) {
		return func(io.Writer, Context) error {
			return errors.New("current caller error")
		}, nil
	})
	cfg.EnableFileCache()

	for _, test := range []struct {
		path string
		line int
	}{
		{path: "first.liquid", line: 10},
		{path: "second.liquid", line: 20},
	} {
		root, err := cfg.Compile(`{% render_file partial %}`, parser.SourceLoc{
			Pathname: test.path,
			LineNo:   test.line,
		})
		require.NoError(t, err)
		err = Render(root, io.Discard, map[string]any{}, cfg)
		require.Error(t, err)
		require.Equal(t, test.path, err.Path())
		require.Equal(t, test.line, err.LineNumber())
	}
}


func TestRenderFileCacheRetriesLoadAndParseFailures(t *testing.T) {
	t.Run("read error", func(t *testing.T) {
		cfg := NewConfig()
		store := &countingTemplateStore{errs: map[string]error{"partial": errors.New("read failure")}}
		cfg.TemplateStore = store
		addFileCacheTestTag(&cfg)
		cfg.EnableFileCache()

		root, err := cfg.Compile(`{% render_file partial %}`, parser.SourceLoc{})
		require.NoError(t, err)
		for range 2 {
			require.Error(t, Render(root, io.Discard, map[string]any{}, cfg))
		}
		require.Equal(t, int32(2), store.reads.Load())
	})

	t.Run("parse error", func(t *testing.T) {
		cfg := NewConfig()
		store := &countingTemplateStore{templates: map[string][]byte{"partial": []byte(`{% undefined_tag %}`)}}
		cfg.TemplateStore = store
		addFileCacheTestTag(&cfg)
		cfg.EnableFileCache()

		root, err := cfg.Compile(`{% render_file partial %}`, parser.SourceLoc{})
		require.NoError(t, err)
		for range 2 {
			require.Error(t, Render(root, io.Discard, map[string]any{}, cfg))
		}
		require.Equal(t, int32(2), store.reads.Load())
	})
}

func TestRenderFileCacheRendersRuntimeErrorsInCurrentContext(t *testing.T) {
	cfg := NewConfig()
	store := &countingTemplateStore{templates: map[string][]byte{"partial": []byte(`{% runtime_error %}`)}}
	cfg.TemplateStore = store
	addFileCacheTestTag(&cfg)
	cfg.AddTag("runtime_error", func(string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			if c.Get("fail") == true {
				return errors.New("runtime failure")
			}
			_, err := io.WriteString(w, "ok")
			return err
		}, nil
	})
	cfg.EnableFileCache()

	root, err := cfg.Compile(`{% render_file partial %}`, parser.SourceLoc{})
	require.NoError(t, err)
	require.Error(t, Render(root, io.Discard, map[string]any{"fail": true}, cfg))

	buf := new(bytes.Buffer)
	require.NoError(t, Render(root, buf, map[string]any{"fail": false}, cfg))
	require.Equal(t, "ok", buf.String())
	require.Equal(t, int32(1), store.reads.Load())
}

func TestRenderFileCacheAllowsNestedReentry(t *testing.T) {
	cfg := NewConfig()
	store := &countingTemplateStore{
		templates: map[string][]byte{"partial": []byte(`{% file_cache_compile %}{% reenter_file partial %}`)},
	}
	var compiles atomic.Int32
	cfg.TemplateStore = store
	addFileCacheCompileCounter(&cfg, &compiles)
	cfg.AddTag("reenter_file", func(filename string) (func(io.Writer, Context) error, error) {
		return func(w io.Writer, c Context) error {
			depth, _ := c.Get("depth").(int)
			if depth > 0 {
				_, err := io.WriteString(w, "done")
				return err
			}
			s, err := c.RenderFile(filename, map[string]any{"depth": depth + 1})
			if err != nil {
				return err
			}
			_, err = io.WriteString(w, s)
			return err
		}, nil
	})
	cfg.EnableFileCache()

	root, err := cfg.Compile(`{% reenter_file partial %}`, parser.SourceLoc{Pathname: "layout.liquid", LineNo: 1})
	require.NoError(t, err)
	buf := new(bytes.Buffer)
	require.NoError(t, Render(root, buf, map[string]any{}, cfg))
	require.Equal(t, "done", buf.String())
	require.Equal(t, int32(2), store.reads.Load())
	require.Equal(t, int32(2), compiles.Load())
}
func TestRenderFileCacheLoadsDifferentKeysConcurrently(t *testing.T) {
	cfg := NewConfig()
	store := &blockingTemplateStore{
		started: make(chan string, 2),
		release: map[string]chan struct{}{
			"first":  make(chan struct{}),
			"second": make(chan struct{}),
		},
	}
	cfg.TemplateStore = store
	addFileCacheTestTag(&cfg)
	cfg.EnableFileCache()

	rootFirst, err := cfg.Compile(`{% render_file first %}`, parser.SourceLoc{Pathname: "layout.liquid", LineNo: 1})
	require.NoError(t, err)
	rootSecond, err := cfg.Compile(`{% render_file second %}`, parser.SourceLoc{Pathname: "layout.liquid", LineNo: 1})
	require.NoError(t, err)

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(store.release["first"])
			close(store.release["second"])
		})
	}
	defer release()

	results := make(chan error, 2)
	go func() {
		results <- Render(rootFirst, io.Discard, map[string]any{}, cfg)
	}()
	go func() {
		results <- Render(rootSecond, io.Discard, map[string]any{}, cfg)
	}()

	seen := make(map[string]bool, 2)
	for range 2 {
		select {
		case filename := <-store.started:
			seen[filename] = true
		case <-time.After(time.Second):
			t.Fatal("different file cache keys were serialized")
		}
	}
	require.True(t, seen["first"])
	require.True(t, seen["second"])

	release()
	require.NoError(t, <-results)
	require.NoError(t, <-results)
}


func TestRenderFileCacheSingleFlightsConcurrentRenders(t *testing.T) {
	cfg := NewConfig()
	store := &countingTemplateStore{
		templates: map[string][]byte{"partial": []byte(`{% file_cache_compile %}{{ page }}:{{ include }}`)},
	}
	var compiles atomic.Int32
	cfg.TemplateStore = store
	addFileCacheTestTag(&cfg)
	addFileCacheCompileCounter(&cfg, &compiles)
	cfg.EnableFileCache()

	root, err := cfg.Compile(`{% render_file partial %}`, parser.SourceLoc{Pathname: "layout.liquid", LineNo: 1})
	require.NoError(t, err)

	type result struct {
		output string
		err    error
	}
	const renders = 32
	start := make(chan struct{})
	results := make(chan result, renders)
	var wg sync.WaitGroup
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			buf := new(bytes.Buffer)
			err := Render(root, buf, map[string]any{
				"page":  fmt.Sprintf("page-%d", i),
				"value": fmt.Sprintf("include-%d", i),
			}, cfg)
			results <- result{output: buf.String(), err: err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	expected := make(map[string]bool, renders)
	for i := range renders {
		expected[fmt.Sprintf("page-%d:include-%d", i, i)] = true
	}
	for result := range results {
		require.NoError(t, result.err)
		require.True(t, expected[result.output], "unexpected output %q", result.output)
		delete(expected, result.output)
	}
	require.Empty(t, expected)
	require.Equal(t, int32(1), store.reads.Load())
	require.Equal(t, int32(1), compiles.Load())
}
