package sqldb

//type Response struct {
//	//【status】:
//	//200: OK       400: Bad Request        500：Internal Server Error
//	//401：Unauthorized
//	//403：Forbidden
//	//404：Not Found
//	Status int    `json:"status" example:"200"` // 【status】
//	Data   Data   `json:"data" example:""`      //
//	Msg    string `json:"msg" example:"success"`
//}

type Response struct {
	// 40003, " %s illegal media file type"
	//
	Code    int         `json:"code" example:1` // 0:
	Data    interface{} `json:"data" example:""`
	Message string      `json:"message" example:""` //
}

// TableName returns name of table
func (r *Response) ReturnFailureString(str string) {
	//r.Status = 200
	//r.Msg = str
	//r.Data = Data{
	//	Code:    -1,
	//	Message: str,
	//}
	r.Code = -1
	r.Message = str
}

// TableName returns name of table
func (r *Response) Return(dat interface{}, ct CodeType) {
	r.Code = ct.Code
	r.Message = ct.Message
	r.Data = dat
}
