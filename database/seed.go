package database

import (
	"database/sql"
	"strings"

	"ofo/logger"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
)

func Seed(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		logger.Info("Database already seeded, skipping")
		return nil
	}

	logger.Info("Seeding database with 摸鱼 sample data...")

	// Insert categories — 按摸鱼场景分类
	categories := map[string]string{
		"速览":   "quick-peek",
		"摸一会":  "bathroom-break",
		"午休档":  "lunch-break",
		"今日精选": "daily-highlight",
	}
	for name, slug := range categories {
		if _, err := db.Exec("INSERT IGNORE INTO categories (name, slug) VALUES (?, ?)", name, slug); err != nil {
			return err
		}
	}

	// Insert tags — 按内容题材分类
	tags := map[string]string{
		"程序员":   "cheng-xu-yuan",
		"打工人":   "da-gong-ren",
		"甲方乙方":  "jia-fang-yi-fang",
		"社死现场":  "she-si-xian-chang",
		"离谱设计":  "li-pu-she-ji",
		"动物":    "dong-wu",
		"互联网考古": "hu-lian-wang-kao-gu",
	}
	for name, slug := range tags {
		if _, err := db.Exec("INSERT IGNORE INTO tags (name, slug) VALUES (?, ?)", name, slug); err != nil {
			return err
		}
	}

	policy := bluemonday.UGCPolicy()

	posts := []struct {
		Title    string
		Slug     string
		Content  string
		Category string
		Tags     []string
	}{
		{
			Title:    "产品经理又提了个离谱需求",
			Slug:     "chan-pin-jing-li-you-ti-liao-ge-li-pu-xu-qiu",
			Category: "bathroom-break",
			Tags:     []string{"程序员", "甲方乙方", "离谱设计"},
			Content: `## 场景还原

周五下午 5:55，你正准备收拾东西下班。产品经理端着咖啡走过来：

> "那个，用户说我们的 App 打开太慢了，能不能让它秒开？"

你刚想说「我们已经优化到 1.2 秒了」，他又补充了一句：

> "就是那个，页面上所有的数据也要提前加载好，用户点任何按钮都能瞬间跳转。对了，后台最好能预测用户下一步想干什么。"

## 经典需求集锦

产品圈的经典语录，你听过几句？

### 1. "这个需求很简单"

*「简单」* = 重构整个后端架构
*「小改动」* = 改 47 个页面
*「就加个按钮」* = 需要新开一张表、写一套 API、改前端三个组件

### 2. "我看竞品有这个功能"

竞品有 500 人的研发团队，你们有 3 个后端 + 1.5 个前端（那个 0.5 还在兼职运维）。

### 3. "能不能做成百度那样的？"

这是最危险的一句话。接下来你会听到关键词：智能推荐、千人千面、大数据分析。

## 程序员生存法则

1. **永远不要在周五下午部署** — 除非你想周末加班
2. **QA 说"我随便点点"的时候，请保持警觉** — 他能点出八个你没想到的 bug
3. **"线上没问题"这句话有魔力** — 说出来就等于在召唤事故
4. **产品经理说"就最后一个需求"** — 这不是最后一个，永远不会是

## 小编点评

当产品经理说"这个需求很简单"的时候，建议你回复："那你写个 demo 给我看看？" —— 这句话能为你争取至少三天的清净时间。`,
		},
		{
			Title:    "同事在周会上不小心共享了屏幕",
			Slug:     "tong-shi-zai-zhou-hui-shang-bu-xiao-xin-gong-xiang-liao-ping-mu",
			Category: "lunch-break",
			Tags:     []string{"社死现场", "打工人"},
			Content: `## 事发经过

周二早上 10 点，全组周会。技术负责人老张在投屏讲 Q3 技术规划。

突然他的微信弹窗出现在大屏幕上——

> **老婆**：你今天出门前是不是没冲厕所？

整个会议室安静了整整三秒钟。那是比任何技术难题都长的三秒钟。

## 远程会议生存法则

疫情之后，在家开视频会的翻车现场层出不穷：

### 经典案例 1：上半身西装，下半身花裤衩

你以为只露出上半身就安全了。直到快递小哥按门铃，你站起来的一瞬间……

### 经典案例 2：以为已经静音了

「这个方案也太菜了吧」—— 你的这句话通过 Zoom 传到了方案提出者本人耳中。

### 经典案例 3：虚拟背景翻车

你精心设置的虚拟背景，在你转头拿杯子的时候，识别算法把你的脖子也替换成了蓝天白云。

## 会议保命清单

- ✅ 共享屏幕前，关闭微信、QQ、钉钉、飞书、Slack、Teams……
- ✅ 检查浏览器标签页（那个"面试技巧"的搜索记录……）
- ✅ 确认已静音（哪怕你以为已经静音了）
- ✅ 穿好裤子（远程会议第一铁律）
- ✅ 虚拟背景靠得住，但别完全依赖它

## 小编点评

老张后来在群里说了一句话：「没事，至少不是在跟甲方开会的时候弹出的。」这句话让他重新获得了同事们的尊重。真正的社死，永远在下一场会议里等着你。`,
		},
		{
			Title:    "这只猫写代码比我快",
			Slug:     "zhe-zhi-mao-xie-dai-ma-bi-wo-kuai",
			Category: "quick-peek",
			Tags:     []string{"动物", "程序员"},
			Content: `## 程序员的终极内卷对手

*（图片：一只猫趴在键盘上，屏幕上出现了一串合法的 Python 代码）*

办公室的编程猫又火了。这只名叫 Glitch 的橘猫在主人离开时，趴在 MacBook 键盘上睡了一觉。主人回来发现，这只猫「写」了 47 行 Python 代码。

最离谱的是：代码居然能跑。

## 实测

主人把代码跑了一下，发现是一段图片处理的脚本——恰好主人前一天在写这个功能。猫踩键盘的过程莫名其妙补全了缺失的参数。

程序员们集体破防：

> 「我读了四年计算机，不如一只猫踩键盘。」

> 「建议把 Glitch 的 GitHub 贡献图挂出来。」

> 「所以这就是传说中的『cat』命令？」

## 网友神评

> 「这只猫的代码肯定没有 bug——它根本不需要写注释，因为只有上帝和猫自己知道代码在干什么。」

> 「建议 Code Review。我赌这猫没写单元测试。」

## 小编点评

当你花了一上午 debug 的时候，一只猫已经超越了你的生产力。建议各位程序员在工位上养一只猫。代码质量不一定提升，但至少能帮你吸引同事来围观——增加了社交价值。`,
		},
		{
			Title:    "甲方说「很简单」的时候",
			Slug:     "jia-fang-shuo-hen-jian-dan-de-shi-hou",
			Category: "bathroom-break",
			Tags:     []string{"甲方乙方", "打工人"},
			Content: `## 危险信号识别

甲方说的每一个平凡词汇，都有另一层含义。来对个暗号：

| 甲方说的 | 实际意思 |
|----------|----------|
| "很简单" | 我不知道怎么实现，但我觉得你应该能搞定 |
| "调一下" | 要把整个设计翻一遍 |
| "高端一点" | 加个渐变、阴影、动画，反正要多点东西 |
| "参考一下" | 我要你做一个一模一样但法律上不算抄袭的东西 |
| "先做着，后面再定" | 你做好了我会说"不是我想要的" |
| "预算有限" | 要用 500 块钱做 500 万的效果 |

## 五阶段情绪变化

和甲方合作的过程，本质上是一个情绪过山车：

1. **兴奋期**：「这次是个好项目！甲方很有想法！」
2. **困惑期**：「等等，他的意思是……要做 NFT + 元宇宙 + AI？」
3. **愤怒期**：「这是第七版方案了，他又改回了第一版！？」
4. **麻木期**：「好的，改。好的，再改。好的……」
5. **超脱期**：「您说什么就是什么吧，我都行。」

## 最后的倔强

甲方："你这个设计……感觉少了点『灵魂』。"

你内心：「灵魂？」
你嘴上：「您说得对，我再调整一下配色和氛围。」

所谓的「灵魂」，翻译过来就是：**我自己也不知道要什么，但你必须做出来让我看了觉得对。**

## 小编点评

当你学会在甲方说"很简单"的时候保持微笑，你就已经从初级乙方升级为资深乙方了。这个行业最核心的能力不是技术，是**情绪管理**。`,
		},
		{
			Title:    "厕所里的神秘代码",
			Slug:     "ce-suo-li-de-shen-mi-dai-ma",
			Category: "lunch-break",
			Tags:     []string{"程序员", "互联网考古"},
			Content: `## 互联网遗迹

有人在公司厕所隔间的门板上，发现了一行用马克笔写的文字：

` + "```\n" + `rm -rf /  # 这是最后的手段
` + "```\n\n" + `下面是另一行笔迹，显然来自后来的某个人：

` + "```\n" + `你不用 sudo 是删不掉的，菜鸡
` + "```\n\n" + `第三个笔迹：

` + "```\n" + `你们就不能好好写文档吗？
` + "```\n\n" + `## 互联网考古学：那些年我们挖出来的神贴

### 1. StackOverflow の「删除法国」

一个用户问：「如何在 JavaScript 中判断时区？」

下面最高赞回答：「首先，你要知道法国在哪个时区……」

下面有人回复：「为什么我们要删除法国？」

原答主：「什么？我没说要删除法国啊。」

——这是 StackOverflow 上最著名的「错别字引发的国际事件」。

### 2. 产品经理 vs 工程师的史诗级 GitHub Issue

某个开源项目里，有人提了个 Issue：「能不能让这个按钮更大一点？」

开发者正儿八经回复：「按钮的大小是 W3C 标准建议的 44px 触摸交互最小尺寸乘以 1.2 倍的人机工程学黄金比例……」

楼下另一个开发者：「他就是想让你加个 <code>padding: 20px</code>。」

### 3. 最古老的"hello world"

1974 年，《C 程序设计语言》的作者 Brian Kernighan 在一份内部备忘录里第一次用了 <code>"hello, world"</code> 作为示例输出。

半个世纪后，全世界每个程序员的第一行代码都是这句话。而它的前身其实是 Kernighan 在 B 语言教程里写的一个叫 <code>hello</code> 的程序……那程序里根本没有 <code>"world"</code>。

## 小编点评

互联网考古最有意思的地方在于：当年写下那些东西的人，根本不知道自己正在创造 meme。厕所门板上的 <code>rm -rf /</code> 和 GitHub 上的第一条 issue 一样，都是数字时代的岩画。建议各位以后上厕所带支笔。`,
		},
	}

	// Insert posts
	for _, p := range posts {
		// Render markdown to HTML
		unsafe := blackfriday.Run([]byte(p.Content))
		html := string(policy.SanitizeBytes(unsafe))

		// Generate excerpt (first 200 chars of plain text)
		excerpt := extractExcerpt(p.Content, 200)

		// Get category ID
		var catID sql.NullInt64
		err := db.QueryRow("SELECT id FROM categories WHERE slug = ?", p.Category).Scan(&catID)
		if err != nil {
			catID = sql.NullInt64{}
		}

		result, err := db.Exec(
			`INSERT INTO posts (title, slug, excerpt, content_md, content_html, category_id, is_published)
			 VALUES (?, ?, ?, ?, ?, ?, 1)`,
			p.Title, p.Slug, excerpt, p.Content, html, catID,
		)
		if err != nil {
			return err
		}

		postID, _ := result.LastInsertId()

		// Insert post_tags
		for _, tagSlug := range p.Tags {
			var tagID int64
			if err := db.QueryRow("SELECT id FROM tags WHERE slug = ?", tagSlug).Scan(&tagID); err != nil {
				continue
			}
			db.Exec("INSERT IGNORE INTO post_tags (post_id, tag_id) VALUES (?, ?)", postID, tagID)
		}
	}

	logger.Info("Seeded 摸鱼 posts successfully", "count", len(posts))
	return nil
}

func extractExcerpt(md string, maxLen int) string {
	// Remove markdown syntax roughly
	md = strings.ReplaceAll(md, "`", "")
	md = strings.ReplaceAll(md, "#", "")
	md = strings.ReplaceAll(md, "*", "")
	md = strings.ReplaceAll(md, "_", "")
	md = strings.ReplaceAll(md, "[", "")
	md = strings.ReplaceAll(md, "]", "")
	md = strings.ReplaceAll(md, "(", "")
	md = strings.ReplaceAll(md, ")", "")
	md = strings.ReplaceAll(md, "```", "")

	// Normalize whitespace
	md = strings.Join(strings.Fields(md), " ")

	if len(md) > maxLen {
		// Try to break at a word boundary
		cut := md[:maxLen]
		if lastSpace := strings.LastIndex(cut, " "); lastSpace > 0 {
			cut = cut[:lastSpace]
		}
		return cut + "..."
	}
	return md
}
