package admin

import (
	"ofo/handlers"
	"ofo/middleware"

	"github.com/gin-gonic/gin"
)

// AdminHandler holds dependencies for admin route handlers.
type AdminHandler struct {
	*handlers.Handler
}

// SetupRoutes registers all admin routes on the given Gin engine.
func SetupRoutes(r *gin.Engine, h *handlers.Handler) {
	a := &AdminHandler{Handler: h}

	admin := r.Group("/admin")

	// 无需认证
	admin.GET("/login", a.AdminLoginPage)
	admin.POST("/login", a.AdminLogin)

	// 需要认证
	protected := admin.Group("")
	protected.Use(middleware.AdminAuth(h.Cfg.AdminPassword))
	{
		protected.GET("/", a.AdminDashboard)                            // 仪表盘
		protected.GET("/posts/new", a.AdminNewPost)                     // 新建文章
		protected.POST("/posts", a.AdminCreatePost)                     // 保存文章
		protected.GET("/posts/:id/edit", a.AdminEditPost)               // 编辑文章
		protected.POST("/posts/:id", a.AdminUpdatePost)                 // 更新文章
		protected.POST("/posts/:id/delete", a.AdminDeletePost)          // 删除文章
		protected.GET("/posts/quick", a.AdminQuickPublish)              // 快速发布表单
		protected.POST("/posts/quick", a.AdminQuickCreatePost)          // 保存快速发布
		protected.GET("/categories", a.AdminCategories)                 // 分类管理
		protected.POST("/categories", a.AdminCreateCategory)            // 新建分类
		protected.POST("/categories/:id", a.AdminUpdateCategory)        // 更新分类
		protected.POST("/categories/:id/delete", a.AdminDeleteCategory) // 删除分类
		protected.POST("/tags/create", a.AdminCreateTagAjax)            // AJAX 新增标签
		protected.GET("/tags", a.AdminTags)                             // 标签管理
		protected.GET("/tags/:id/posts", a.AdminTagPosts)               // 标签关联文章
		protected.POST("/tags/:id/update", a.AdminUpdateTag)            // 重命名标签
		protected.POST("/tags/:id/delete", a.AdminDeleteTag)            // 删除标签
		protected.GET("/series", a.AdminSeries)                         // 系列管理
		protected.POST("/series", a.AdminCreateSeries)                  // 新建系列
		protected.GET("/series/:id/posts", a.AdminSeriesPosts)          // 系列文章
		protected.POST("/series/:id/update", a.AdminUpdateSeries)       // 重命名
		protected.POST("/series/:id/delete", a.AdminDeleteSeries)       // 删除
		protected.POST("/series/:id/order", a.AdminUpdateSeriesOrder)   // 排序
		protected.GET("/resources", a.AdminResources)                   // 资源管理
		protected.POST("/resources/:id/delete", a.AdminDeleteResource)  // 删除资源
		protected.POST("/upload", a.AdminUpload)                        // 文件上传

		protected.POST("/resolve-content", a.AdminResolveContent) // 编辑器预览 URL 解析
		protected.GET("/logout", a.AdminLogout)                   // 退出登录
	}
}
