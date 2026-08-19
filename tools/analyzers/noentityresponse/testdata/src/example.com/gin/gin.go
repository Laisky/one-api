package gin

// H mirrors gin.H (map[string]any).
type H map[string]any

// Context mirrors *gin.Context's JSON responders (only the signatures matter).
type Context struct{}

func (c *Context) JSON(code int, obj any)                {}
func (c *Context) IndentedJSON(code int, obj any)        {}
func (c *Context) PureJSON(code int, obj any)            {}
func (c *Context) AbortWithStatusJSON(code int, obj any) {}
