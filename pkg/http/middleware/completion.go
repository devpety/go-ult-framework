package middleware

import (
	"sync"

	"github.com/gin-gonic/gin"
)

const (
	requestCompletionKey      = "ult.request_completion"
	requestCompletionOwnerKey = "ult.request_completion_owner"
)

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

// ManageRequestCompletion 将收尾交给最外层调用者，避免内层提前输出和回收 Context。
// 自定义 Recovery 可在执行下游前调用，并在收尾阶段调用 CompleteRequest。
func ManageRequestCompletion(ctx *gin.Context) {
	ctx.Set(requestCompletionOwnerKey, true)
}
