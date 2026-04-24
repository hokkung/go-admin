package embedded_cycle

// Node embeds itself transitively via Inner. Not legal Go for actual
// allocation, but the AST accepts it and we want to prove the cycle guard
// in collectFields stops the recursion rather than overflow the stack.
type Node struct {
	Name string
	Inner
}

type Inner struct {
	Node
}

//admin:generate
type Thing struct {
	ID   uint   `json:"id" admin:"id"`
	Name string `json:"name"`
	Node
}
