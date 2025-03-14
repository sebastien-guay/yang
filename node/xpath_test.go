package node_test

import (
	"testing"

	"github.com/freeconf/yang/fc"
	"github.com/freeconf/yang/node"
	"github.com/freeconf/yang/nodeutil"
	"github.com/freeconf/yang/parser"
	"github.com/freeconf/yang/xpath"
)

func TestXFind(t *testing.T) {
	fc.DebugLog(true)
	mstr := ` module m { namespace ""; prefix ""; revision 0; 
		container a {
			leaf b {
				type int32;
			}
		}
		container aa {
			leaf bb {
				type string;
			}
		}
		container aaa {
			leaf bbb {
				type boolean;
			}
		}		
		list list {
			leaf leaf {
				type int32;
			} 
		}
	}
	`
	m, err := parser.LoadModuleFromString(nil, mstr)
	if err != nil {
		t.Fatal(err)
	}
	b := node.NewBrowser(m, nodeutil.ReadJSON(`{
		"a":{"b":10},
		"aa":{"bb":"hello"},
		"aaa":{"bbb":true},
		"list":[{"leaf":99},{"leaf":100}]
	}`))
	tests := []struct {
		xpath    string
		expected string
	}{
		{
			xpath:    `a/b<20`,
			expected: `{"b":10}`,
		},
		{
			xpath: `a/b<2`,
		},
		{
			xpath: `a/b!=10`,
		},
		{
			xpath:    `a/b=10`,
			expected: `{"b":10}`,
		},
		{
			xpath:    `aa/bb='hello'`,
			expected: `{"bb":"hello"}`,
		},
		{
			xpath:    `list/leaf=99`,
			expected: `{"leaf":99}`,
		},
		{
			xpath: `aa/bb!='hello'`,
		},
		{
			xpath:    `aaa/bbb='true'`,
			expected: `{"bbb":true}`,
		},
	}
	for _, test := range tests {
		p, err := xpath.Parse(test.xpath)
		if err != nil {
			t.Error(err)
		}
		s := b.Root().XFind(p)
		if s.LastErr != nil {
			t.Error(s.LastErr)
		} else if test.expected != "" {
			if s.IsNil() {
				t.Error("not found but expected to find ", test.expected)
			} else {
				actual, _ := nodeutil.WriteJSON(s)
				fc.AssertEqual(t, test.expected, actual)
			}
		} else if !s.IsNil() {
			actual, _ := nodeutil.WriteJSON(s)
			t.Errorf("expected no results from %s but found %s", test.xpath, actual)
		}
	}
}
