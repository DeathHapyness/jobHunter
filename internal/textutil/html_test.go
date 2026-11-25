package textutil

import "testing"

func TestHTMLToText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"vazio", "", ""},
		{"simples", "<p>Olá <b>mundo</b></p><p>Segundo</p>", "Olá mundo\nSegundo"},
		{"escapado", "&lt;p&gt;A &amp;amp; B&lt;/p&gt;", "A & B"},
		{"script", "<p>x</p><script>alert(1)</script><p>y</p>", "x\ny"},
		{"lista", "<ul><li>um</li><li>dois</li></ul>", "um\ndois"},
		{"nbsp", "a&nbsp;&nbsp;b", "a b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HTMLToText(c.in); got != c.want {
				t.Fatalf("HTMLToText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
