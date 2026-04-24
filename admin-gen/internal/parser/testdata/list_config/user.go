package list_config

//admin:generate default_page_size=25 max_page_size=200 default_sort="created_at:desc,name:asc"
type User struct {
	ID        uint   `json:"id" admin:"id"`
	Name      string `json:"name" admin:"sortable"`
	CreatedAt string `json:"created_at" admin:"sortable"`
}
