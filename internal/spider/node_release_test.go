package spider

import "testing"

func TestReleaseOtherKeepsActivePkgWhenAPIUnindexed(t *testing.T) {
	h := &NodeHost{pkgs: map[string]*nodePkg{}}
	only := &nodePkg{id: "only", port: 12345}
	h.pkgs[only.id] = only
	// Node api 尚未进 apiIndex 时仍应保留当前已起包，勿全杀后冷启。
	h.ReleaseOther("/spider/unknown/3")
	if only.port != 12345 {
		t.Fatal("should keep sole active pkg when api unindexed")
	}
}

func TestReleaseOtherStopsAllForNonNodeAPI(t *testing.T) {
	h := &NodeHost{pkgs: map[string]*nodePkg{}}
	a := &nodePkg{id: "a", port: 1}
	b := &nodePkg{id: "b", port: 2}
	h.pkgs[a.id] = a
	h.pkgs[b.id] = b
	h.ReleaseOther("csp_Foo")
	if a.port != 0 || b.port != 0 {
		t.Fatal("non-node api should stop all node pkgs")
	}
}

func TestReleaseOtherKeepsIndexedPkg(t *testing.T) {
	h := &NodeHost{
		pkgs:     map[string]*nodePkg{},
		apiIndex: map[string]string{"/spider/a/3": "pkgA"},
	}
	a := &nodePkg{id: "pkgA", port: 11}
	b := &nodePkg{id: "pkgB", port: 22}
	h.pkgs[a.id] = a
	h.pkgs[b.id] = b
	h.ReleaseOther("/spider/a/3")
	if a.port != 11 {
		t.Fatal("indexed keep api should retain pkg")
	}
	if b.port != 0 {
		t.Fatal("other pkg should stop")
	}
}
