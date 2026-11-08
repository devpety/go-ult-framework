package errcode

var zhCNText = map[int]string{
	ServerError:              "内部服务器错误",
	ServiceUnavailableError:  "服务暂不可用",
	RequestBodyTooLargeError: "请求体超过限制",
	RequestTimeoutError:      "请求读取超时",
	AuthorizationError:       "签名信息错误",
	ParamBindError:           "参数信息错误",
	RequestError:             "请求错误",
	ParamValidateError:       "参数校验错误",
	UnknownError:             "未知错误",
	DataNotExistError:        "数据不存在",
	DataExistError:           "数据已存在",
	RequestNotFoundError:     "不存在的请求",
	DataDeleteError:          "数据删除错误",
	ResourceNotExistError:    "资源不存在",
	DataSelectError:          "数据查询失败",
	DataCreateError:          "数据创建失败",
	DataUpdateError:          "数据更新失败",
}
