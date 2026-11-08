package middleware

import (
	"sync"

	"github.com/gin-gonic/gin"
)

const requestCompletionKey = "ult.request_completion"

type requestCompletion struct {
	once     sync.Once
	response func()
}

// CompleteRequest 执行 Request 登记的响应回调，每个请求最多执行一次。
// 替换内置 Recovery 时，应在异常转换完成后调用；正常路径也需要调用。
func CompleteRequest(ctx *gin.Context) {
	value, exists := ctx.Get(requestCompletionKey)
	if !exists {
		return
	}
	completion, ok := value.(*requestCompletion)
	if !ok {
		return
	}
	completion.once.Do(func() {
		// 先移除入口，响应回调再次触发收尾时不会重入同一个 Once。
		ctx.Set(requestCompletionKey, nil)
		if completion.response != nil {
			completion.response()
		}
	})
}
