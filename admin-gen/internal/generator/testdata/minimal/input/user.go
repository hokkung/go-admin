package minimal

//admin:generate
type User struct {
	ID   uint   `json:"id" admin:"id,sortable"`
	Name string `json:"name" admin:"filterable,required"`
}
