package scriptstate

import (
	"github.com/Knetic/govaluate"
	lua "github.com/yuin/gopher-lua"
	luar "layeh.com/gopher-luar"
)

type exprState map[string]interface{}

// State object
type State struct {
	expr exprState
	lua  *lua.LState
}

// Init initializes the script state
func (state *State) Init() {
	state.expr = make(exprState, 64)
	state.lua = lua.NewState()
}

// Close closes the Lua state object
func (state *State) Close() {
	state.lua.Close()
}

// SetLua sets a global variable in the Lua state
func (state *State) SetLua(name string, value interface{}) {
	state.lua.SetGlobal(name, luar.New(state.lua, value))
}

// SetExpr sets a global variable in the expression state
func (state *State) SetExpr(name string, value interface{}) {
	state.expr[name] = value
}

// SetAll sets a global variable in both states
func (state *State) SetAll(name string, value interface{}) {
	state.SetExpr(name, value)
	state.SetLua(name, value)
}

// SetBoth sets a global variable in all states to separate values
func (state *State) SetBoth(name string, exprValue interface{}, luaValue interface{}) {
	state.SetExpr(name, exprValue)
	state.SetLua(name, luaValue)
}

// EvalExpr evaluates the expression with the current state
func (state *State) EvalExpr(expr string) (interface{}, error) {
	exp, err := govaluate.NewEvaluableExpression(expr)

	if err != nil {
		return nil, err
	} else {
		res, err := exp.Evaluate(state.expr)

		return res, err
	}
}

// EvalLua evaluates the expression with the current state
func (state *State) EvalLua(lua string) error {
	return state.lua.DoString(lua)
}
