package script

import (
	"testing"
)

// The contract ExContractAs executes, and the contracts it calls, see the account's $key_id and
// $account_id; the caller sees its own again once it returns, and when the contract fails
func TestExContractAs(t *testing.T) {
	vm := NewVM()
	vm.Extend(&ExtendData{
		Objects: map[string]any{
			"CallAs": func(rt *RunTime, name string) (any, error) {
				return ExContractAs(rt, 1, name, nil, 5, "0000-0000-0000-0000-0005")
			},
		},
		AutoPars: map[string]string{`*script.RunTime`: `rt`},
	})
	owner := &OwnerInfo{StateID: 1}
	for _, code := range []string{
		`contract AsNested {
			action {
				if $key_id != 5 || $account_id != "0000-0000-0000-0000-0005" {
					error "the nested contract runs as the signer"
				}
			}
		}`,
		`contract AsInner {
			action {
				if $key_id != 5 || $account_id != "0000-0000-0000-0000-0005" {
					error "the contract runs as the signer"
				}
				@1AsNested()
			}
		}`,
		`contract AsFailing {
			action {
				error "failed"
			}
		}`,
		`contract AsOuter {
			action {
				CallAs("@1AsInner")
				if $key_id != 7 || $account_id != "0000-0000-0000-0000-0007" {
					error "the signer's identity is not restored"
				}
			}
		}`,
		`contract AsOuterFailing {
			action {
				CallAs("@1AsFailing")
			}
		}`,
	} {
		if err := vm.Compile([]rune(code), owner); err != nil {
			t.Fatal(err)
		}
	}
	run := func(name string) (map[string]any, error) {
		extend := map[string]any{
			Extend_key_id:     int64(7),
			Extend_account_id: "0000-0000-0000-0000-0007",
			Extend_txcost:     int64(100000),
		}
		obj := vm.Objects[`@1`+name]
		_, err := VMRun(vm, obj.GetCodeBlock().Objects[`action`].GetCodeBlock(), nil, extend, nil)
		return extend, err
	}
	if _, err := run("AsOuter"); err != nil {
		t.Fatal(err)
	}
	extend, err := run("AsOuterFailing")
	if err == nil {
		t.Fatal("a failing contract does not fail its caller")
	}
	if extend[Extend_key_id] != int64(7) || extend[Extend_account_id] != "0000-0000-0000-0000-0007" {
		t.Fatalf("the signer's identity is not restored after a failure: %v %v", extend[Extend_key_id], extend[Extend_account_id])
	}
}
