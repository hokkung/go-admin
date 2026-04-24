package full

//admin:generate default_page_size=25 max_page_size=200 default_sort="created_at:desc"
//admin:action name=activate display="Activate User"
//admin:action name=purge destructive
type User struct {
	ID     uint   `json:"id" admin:"id,sortable"`
	Name   string `json:"name" admin:"filterable,sortable,required" validate:"minLength=1,maxLength=255"`
	Status string `json:"status" admin:"filterable" enum:"active,inactive"`
	Timestamps
}
