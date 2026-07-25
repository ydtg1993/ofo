package database

import (
	"database/sql"
	"strings"
	"time"

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

	logger.Info("Seeding database with sample data...")

	// ---- Categories ----
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

	// ---- Tags ----
	tags := map[string]string{
		"程序员":   "cheng-xu-yuan",
		"打工人":   "da-gong-ren",
		"甲方乙方":  "jia-fang-yi-fang",
		"社死现场":  "she-si-xian-chang",
		"离谱设计":  "li-pu-she-ji",
		"动物":    "dong-wu",
		"互联网考古": "hu-lian-wang-kao-gu",
		"办公室":   "ban-gong-shi",
		"冷笑话":   "leng-xiao-hua",
		"奇闻":    "qi-wen",
	}
	for name, slug := range tags {
		if _, err := db.Exec("INSERT IGNORE INTO tags (name, slug) VALUES (?, ?)", name, slug); err != nil {
			return err
		}
	}

	// ---- Series ----
	series := map[string]string{
		"程序员生存指南": "cheng-xu-yuan-sheng-cun-zhi-nan",
		"甲方图鉴":    "jia-fang-tu-jian",
	}
	for name, slug := range series {
		if _, err := db.Exec("INSERT IGNORE INTO series (name, slug) VALUES (?, ?)", name, slug); err != nil {
			return err
		}
	}

	policy := bluemonday.UGCPolicy()

	type seedPost struct {
		Title    string
		Slug     string
		Content  string
		Category string
		Tags     []string
		Series   string // slug
		DaysAgo  int
	}

	posts := []seedPost{
		{
			Title: "产品经理又提了个离谱需求", Slug: "pm-li-pu-xu-qiu",
			Category: "bathroom-break", Tags: []string{"程序员", "甲方乙方", "离谱设计"},
			DaysAgo: 2,
			Content: `## 场景还原

周五下午 5:55，你正准备收拾东西下班。产品经理端着咖啡走过来：

> "那个，用户说我们的 App 打开太慢了，能不能让它秒开？"

你刚想说「我们已经优化到 1.2 秒了」，他又补充了一句：

> "就是那个，页面上所有的数据也要提前加载好，用户点任何按钮都能瞬间跳转。对了，后台最好能预测用户下一步想干什么。"

## 经典需求集锦

### 1. "这个需求很简单"

「简单」 = 重构整个后端架构。
「小改动」 = 改 47 个页面。
「就加个按钮」 = 新开一张表、写一套 API、改前端三个组件。

### 2. "我看竞品有这个功能"

竞品有 500 人的研发团队，你们有 3 个后端 + 1.5 个前端（那个 0.5 还在兼职运维）。

### 3. "能不能做成百度那样的？"

接下来你会听到关键词：智能推荐、千人千面、大数据分析。

## 程序员生存法则

1. **永远不要在周五下午部署** — 除非你想周末加班
2. **QA 说"我随便点点"的时候保持警觉** — 他能点出八个你没想到的 bug
3. **"线上没问题"这句话有魔力** — 说出来就等于在召唤事故
4. **产品经理说"就最后一个需求"** — 永远不会是最后一个

> 建议回复：「那你写个 demo 给我看看？」—— 能争取至少三天清净。`,
		},
		{
			Title: "同事在周会上不小心共享了屏幕", Slug: "zhou-hui-she-si",
			Category: "lunch-break", Tags: []string{"社死现场", "打工人", "办公室"},
			DaysAgo: 5,
			Content: `## 事发经过

周二早上 10 点，全组周会。技术负责人老张在投屏讲 Q3 技术规划。

突然他的微信弹窗出现在大屏幕上——

> **老婆**：你今天出门前是不是没冲厕所？

整个会议室安静了整整三秒钟。那是比任何技术难题都长的三秒钟。

## 远程会议翻车现场

### 上半身西装，下半身花裤衩

你以为只露出上半身就安全了。直到快递小哥按门铃，你站起来的一瞬间……

### 以为已经静音了

「这个方案也太菜了吧」—— 你的这句话通过 Zoom 传到了方案提出者本人耳中。

### 虚拟背景翻车

你精心设置的虚拟背景，在你转头拿杯子的时候，识别算法把你的脖子也替换成了蓝天白云。

## 会议保命清单

- ✅ 共享屏幕前关闭所有聊天软件
- ✅ 检查浏览器标签页
- ✅ 确认已静音
- ✅ 穿好裤子（远程会议第一铁律）

> 老张后来在群里说：「没事，至少不是在跟甲方开会的时候弹出的。」`,
		},
		{
			Title: "这只猫写代码比我快", Slug: "cat-writes-code",
			Category: "quick-peek", Tags: []string{"动物", "程序员"},
			DaysAgo: 4,
			Content: `办公室的编程猫又火了。这只名叫 Glitch 的橘猫在主人离开时趴在键盘上，结果「写」了 47 行 Python 代码——而且能跑。

主人把代码跑了一下，发现恰好补全了他前一天在写的图片处理脚本的缺失参数。程序员们集体破防：

> 「我读了四年计算机，不如一只猫踩键盘。」

> 「所以这就是传说中的 cat 命令？」

> 「建议 Code Review。我赌这猫没写单元测试。」

当你花了一上午 debug 的时候，一只猫已经超越了你的生产力。建议在工位上养一只——代码质量不一定提升，但社交价值拉满。`,
		},
		{
			Title: "甲方说「很简单」的时候", Slug: "jia-fang-hen-jian-dan",
			Category: "bathroom-break", Tags: []string{"甲方乙方", "打工人"},
			Series:  "jia-fang-tu-jian",
			DaysAgo: 7,
			Content: `## 甲方词典

| 甲方说的 | 实际意思 |
|----------|----------|
| "很简单" | 我不知道怎么实现，但我觉得你应该能搞定 |
| "调一下" | 要把整个设计翻一遍 |
| "高端一点" | 加渐变、阴影、动画，反正要多点东西 |
| "参考一下" | 要做一个一模一样但法律上不算抄袭的东西 |
| "先做着后面再定" | 你做好了我会说"不是我想要的" |
| "预算有限" | 要用 500 块做 500 万的效果 |

## 五阶段情绪过山车

1. **兴奋期**：「这次是个好项目！」
2. **困惑期**：「等等，他要做 NFT + 元宇宙 + AI？」
3. **愤怒期**：「第七版方案了又改回第一版！？」
4. **麻木期**：「好的改，好的再改……」
5. **超脱期**：「您说什么就是什么吧。」

> 当你学会在甲方说"很简单"的时候保持微笑，就已经从初级乙方升级为资深乙方了。这个行业最核心的能力不是技术，是情绪管理。`,
		},
		{
			Title: "厕所里的神秘代码", Slug: "toilet-code",
			Category: "lunch-break", Tags: []string{"程序员", "互联网考古", "冷笑话"},
			DaysAgo: 9,
			Content: `## 互联网遗迹

公司厕所隔间门板上，有人用马克笔写了一行：

` + "```\nrm -rf /  # 这是最后的手段\n```\n\n" + `第二个人回复：

` + "```\n你不用 sudo 是删不掉的，菜鸡\n```\n\n" + `第三个人：

` + "```\n你们就不能好好写文档吗？\n```\n\n" + `## StackOverflow の「删除法国」

一个用户问：「如何在 JavaScript 中判断时区？」最高赞回答开头：「首先，你要知道法国在哪个时区……」

下面有人回复：「为什么我们要删除法国？」—— 这是 StackOverflow 最著名的错别字事件。

## 最古老的 hello world

1974 年 Brian Kernighan 在一份内部备忘录里第一次用了 "hello, world"。半个世纪后，全世界每个程序员的第一行代码都是这句话。

> 厕所门板上的 rm -rf / 和 GitHub 的第一条 issue 一样，都是数字时代的岩画。`,
		},
		{
			Title: "开会时如何假装在做笔记", Slug: "fake-meeting-notes",
			Category: "quick-peek", Tags: []string{"打工人", "办公室", "冷笑话"},
			DaysAgo: 3,
			Content: `## 核心技巧

开会时领导滔滔不绝，你其实一个字都没听进去。但你的笔记本上密密麻麻。

### 写什么？

不要写跟工作有关的内容——万一被看到就露馅了。写这些东西：

- 你最近在看的书的摘抄
- 下周的购物清单（假装是项目排期）
- 编程问题草稿（看起来像在思考技术方案）

### 关键表情管理

每隔 3 分钟抬头看一次投影幕，微微点头。偶尔皱眉表示在「深度思考」。

领导的视线扫过来时，立刻在笔记本上画一个方框加箭头——看起来像在画架构图。

### 什么时候不能装

- 领导点名问你意见时（迅速说「我同意，补充一点……」然后把刚才偷听到的关键词组织成句子）
- 只有三个人的小会（装不了，老实听吧）
- 你的麦克风没关

> 真正的职场高手不是最努力的那个，是最会让别人觉得自己在努力的那个。`,
		},
		{
			Title: "老板让我做 AI 但我只会 if-else", Slug: "ai-is-if-else",
			Category: "quick-peek", Tags: []string{"程序员", "冷笑话"},
			Series:  "cheng-xu-yuan-sheng-cun-zhi-nan",
			DaysAgo: 6,
			Content: "## 问题\n\n老板：「我们要拥抱 AI 时代！下周上线一个智能推荐！」\n\n你看了看自己维护了三年、最复杂的逻辑是三层 if-else 的系统，陷入了沉思。\n\n## 解决方案\n\n```python\ndef ai_recommend(user):\n    if user.vip:\n        return \"推荐：年度会员续费\"\n    elif user.login_days > 365:\n        return \"推荐：老用户回馈礼包\"\n    elif user.last_purchase == \"咖啡\":\n        return \"推荐：提神套餐\"\n    else:\n        return \"推荐：热门商品\"\n```\n\nPPT 上写：「基于用户行为的多维度智能推荐引擎，采用决策树算法实时分析用户画像。」\n\n老板：「很好，这就是我们需要的 AI！」\n\n## 进阶技巧\n\n- if-else 嵌套超过 5 层 → 叫「深度神经网络」\n- switch-case → 叫「专家系统」\n- 随机数 → 叫「蒙特卡洛模拟」\n- 查数据库 → 叫「知识图谱推理」\n\n> 只要命名够唬人，谁在乎你用的是 if-else 还是 GPT-4？",
		},
		{
			Title: "设计师和程序员之间永远解不开的结", Slug: "designer-vs-dev",
			Category: "bathroom-break", Tags: []string{"离谱设计", "程序员", "冷笑话"},
			DaysAgo: 8,
			Content: `## 经典场景

设计师：「这个圆角帮我改成 6px。」

程序员打开 CSS，发现当前是 4px。改完。

设计师：「还是改回 4px 吧，6px 不够精致。」

## 为什么设计师和程序员总打架

| 设计师眼中的世界 | 程序员眼中的世界 |
|-----------------|-----------------|
| 像素级完美 | 差不多就行了 |
| 这个动效很简单 | 你知道这个要用 WebGL 吗 |
| 参考苹果的设计 | 我们有苹果的设计团队吗 |
| 字间距再大 1px | （打开设计稿，发现是 1.37px） |

## 和平共处五项原则

1. 设计师不拿 Sketch 文件说「就照着这个做」
2. 程序员不说「这个做不了」（哪怕真的很难）
3. 改需求要请喝奶茶
4. 周五下午禁止提 UI 改动
5. 双方都承认：用户的手机屏幕跟他们用的 5K 显示器不一样

> 本质上，设计师和程序员都是甲方受害者。应该联合起来。`,
		},
		{
			Title: "2024 年最离谱的技术面试题", Slug: "crazy-interview-2024",
			Category: "daily-highlight", Tags: []string{"程序员", "打工人"},
			DaysAgo: 10,
			Content: `## 真实投稿

> 面试官：「请用纸笔手写一个能在 O(1) 时间内找出数组中位数、同时支持动态扩容、还要线程安全的算法。」

我：「……我能用电脑吗？」

面试官：「不能用电脑，我们这里考验的是基本功。」

## 更多离谱面试题

### 「请解释一下你在梦中是如何调试代码的」

投递岗位：前端开发。面试官说这是考察「元认知能力」。

### 「如果浏览器是一个国家，你会怎么设计它的法律体系？」

面试官很满意自己的创意。候选人默默退出了视频会议。

### 「用三个词描述你为什么适合这个岗位」

候选人写了：「需要钱。有能力。不挑活。」—— 最后居然被录用了。

## 面试官行为大赏

- 「我们这个岗位要 5 年 React 经验」—— React 发布才 11 年，但你要的是 5 年 SSR 经验、3 年 Next.js 13+……
- 「薪资面议」—— 面完发现是 6K
- 「我们不加班」—— 然后面试约在周六下午 3 点

> 找工作是双向选择。面试官在考你的时候，你也在考察这家公司值不值得去。`,
		},
		{
			Title: "如何用三句话激怒一个程序员", Slug: "trigger-a-dev",
			Category: "quick-peek", Tags: []string{"程序员", "冷笑话"},
			DaysAgo: 1,
			Content: `## 三句话

1. 「这个应该很简单吧，不就加个按钮吗？」
2. 「我用 Excel 也能做。」
3. 「为什么淘宝 200 块的模板不能直接用？」

## 更多暴击语录

- 「能不能让这个 App 像微信一样流畅？预算 5000。」
- 「你是程序员对吧？帮我修一下打印机。」
- 「PHP 不是最好的语言吗？」（在 Java 程序员面前说）
- 「空格比 Tab 好。」（在 Python 程序员面前把这两词换着说）
- 「这个 bug 你先别修，我明天再看看。」—— 来自周五下午的产品经理

> 如果你想让一个程序员快速进入战斗状态，只需要说：「你们这个系统架构有问题。」`,
		},
		{
			Title: "地铁上刷手机的正确姿势", Slug: "subway-phone-skills",
			Category: "quick-peek", Tags: []string{"打工人", "冷笑话", "办公室"},
			DaysAgo: 11,
			Content: `## 场景分类

### 刷技术文章

周围都是打工人，你手机屏幕上显示的是《深入理解计算机系统》PDF。旁边的人默默把手机屏幕转向了自己那边——他正在看小说。

### 假装回工作消息

打开企业微信，手指飞速划动但实际上什么都没看。表情严肃，偶尔叹气——完美伪装。

### 刷朋友圈

看到同事发的「又是充实的一天 #加班 #奋斗」，配图是办公室窗户外的夜景。你知道他 5 点半就走了，照片是夏天拍的。

### 看搞笑视频憋笑

这是最高难度的操作。嘴角的肌肉在抽搐，眼泪在打转，但你坚持不发出声音。旁边的大妈以为你中暑了。

## 拥挤程度与手机尺寸的关系

| 拥挤度 | 推荐屏幕尺寸 |
|--------|------------|
| 有空座 | iPad Pro 12.9寸 随便看 |
| 站但能转身 | 6.7寸 双手操作 |
| 人贴人 | 单手模式开启 拇指范围 |
| 罐头模式 | 放弃手机 闭眼冥想 |

> 地铁通勤这门学问，深不可测。`,
		},
		{
			Title: "午饭时间办公室里的终极对决", Slug: "lunch-wars",
			Category: "lunch-break", Tags: []string{"打工人", "办公室", "奇闻"},
			DaysAgo: 12,
			Content: `## 微波炉前的战争

午休 12:00，办公室微波炉只有两台。全公司 80 号人。

### 五大罪状

1. **占着茅坑不拉屎**：叮好了不拿走，下一个人得帮他端出来
2. **热鱼**：整个办公室的人都想杀了你
3. **热榴莲**：比热鱼更恶劣，建议直接开除
4. **不盖盖子**：热完微波炉内壁全是油渍，下一个人的饭遭殃
5. **叮了又叮**：已经热好几分钟了还觉得不够烫，后面的队伍已经排到了电梯口

### 生存策略

- 11:30 偷偷去热饭（错峰出行）
- 带凉面/沙拉（不需要微波炉）
- 加入带饭联盟（每天一个人帮全组热）
- 和行政搞好关系（提前知道哪天微波炉要大清洗）

### 冰箱惨案

公司冰箱每周五清理。没贴标签的、超过一周的、看不出原材料的——全部丢掉。有人丢过限量版蛋糕，有人在群里骂了一下午。

> 办公室午饭看似是个吃饭问题，本质上是资源分配、公共治理和人际关系的高度浓缩。`,
		},
	}

	// Insert posts with staggered dates
	for _, p := range posts {
		unsafe := blackfriday.Run([]byte(p.Content))
		html := string(policy.SanitizeBytes(unsafe))
		excerpt := extractExcerpt(p.Content, 200)

		var catID sql.NullInt64
		db.QueryRow("SELECT id FROM categories WHERE slug = ?", p.Category).Scan(&catID)

		createdAt := time.Now().AddDate(0, 0, -p.DaysAgo).Format("2006-01-02 15:04:05")

		result, err := db.Exec(
			`INSERT INTO posts (title, slug, excerpt, content_md, content_html, category_id, is_published, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 1, ?)`,
			p.Title, p.Slug, excerpt, p.Content, html, catID, createdAt,
		)
		if err != nil {
			return err
		}

		postID, _ := result.LastInsertId()

		// Link tags
		for _, tagSlug := range p.Tags {
			var tagID int64
			if err := db.QueryRow("SELECT id FROM tags WHERE slug = ?", tagSlug).Scan(&tagID); err != nil {
				continue
			}
			db.Exec("INSERT IGNORE INTO post_tags (post_id, tag_id) VALUES (?, ?)", postID, tagID)
		}

		// Link series
		if p.Series != "" {
			var seriesID int64
			if err := db.QueryRow("SELECT id FROM series WHERE slug = ?", p.Series).Scan(&seriesID); err == nil {
				var count int
				db.QueryRow("SELECT COUNT(*) FROM post_series WHERE series_id = ?", seriesID).Scan(&count)
				db.Exec("INSERT INTO post_series (post_id, series_id, sort_order) VALUES (?, ?, ?)",
					postID, seriesID, count+1)
			}
		}
	}

	logger.Info("Seeded successfully", "posts", len(posts))
	return nil
}

func extractExcerpt(md string, maxLen int) string {
	md = strings.ReplaceAll(md, "`", "")
	md = strings.ReplaceAll(md, "#", "")
	md = strings.ReplaceAll(md, "*", "")
	md = strings.ReplaceAll(md, "_", "")
	md = strings.ReplaceAll(md, "[", "")
	md = strings.ReplaceAll(md, "]", "")
	md = strings.ReplaceAll(md, "(", "")
	md = strings.ReplaceAll(md, ")", "")
	md = strings.ReplaceAll(md, "```", "")
	md = strings.Join(strings.Fields(md), " ")

	if len(md) > maxLen {
		cut := md[:maxLen]
		if lastSpace := strings.LastIndex(cut, " "); lastSpace > 0 {
			cut = cut[:lastSpace]
		}
		return cut + "..."
	}
	return md
}
