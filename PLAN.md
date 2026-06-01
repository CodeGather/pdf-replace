# pdf-replace 开发计划

> 每完成一个节点，将标记由 `[ ]` 改为 `[x]`，并记录完成日期。

---

## 阶段一：项目骨架搭建

### [x] 1.1 初始化 Go 项目 (2026-06-01)
- `go mod init pdf-replace`
- 建立目录结构（`cmd/`, `config/`, `matcher/`, `pdf/`, `model/`, `parallel/`, `assets/`）
- 嵌入 MicrosoftYaHei.ttf 中文字体到 `assets/`
- 搭建 `main.go` CLI 入口（flag 解析+帮助信息）

### [x] 1.2 数据模型定义 (2026-06-01)
- 在 `model/types.go` 中定义所有 Go 结构体
- 包括：`Config`, `TableColumn`, `BrandConfig`, `LampItem`, `DbData`, `DbLamp`, `FileData`, `FileEntry`, `ImageMeta` 等
- JSON 标签、嵌套结构、`编号列表` JSON 字符串自定义反序列化

### [x] 1.3 JSON 解析 + 数据校验 (2026-06-01)
- 在 `config/json.go` 中实现 JSON 文件读取和解析
- 基础校验：必填字段存在、类型正确、`lamp` 数组不为空
- 验证通过：实际 `1.json` 解析成功（10灯位、10素材、10编号列表条目）

---

## 阶段二：素材匹配引擎

### [x] 2.1 方向 + 宽高比匹配算法 + 单元测试 (2026-06-01)
- 在 `matcher/matcher.go` 中实现核心算法：方向判定、单图直接使用、横/竖/正方形匹配、无同方向兜底
- 9 个单元测试全部通过

### [x] 2.2 灯位备注 → file-data 模糊匹配 (2026-06-01)
- 实现 key 清理（去空格、全角转半角、转小写）
- 精确匹配 + Levenshtein 距离 ≤ 2 模糊匹配

---

## 阶段三：PDF 图片替换（单灯位）

### [x] 3.1 模板 PDF 打开 + 图片定位 (2026-06-01)
- 使用 pdfcpu 打开模板 PDF，解析 XObject 字典
- 修复：`Dereference` 解引用 Resources/XObject 避免类型断言失败
- 修复：`Decode()` 解压 FlateDecode 内容流
- 修复：tokenizer 中 readingName 标识避免 `/Im0` 被拆分为 `Im`+`0`
- 修复：`cm` 操作符的错误 `i+=6` 跳跃导致跳过 `Do` 操作符
- 从模板 PDF 正确提取 11 张图片的位置（objNr, 坐标, 尺寸）

### [x] 3.2 图片替换 + contain 适配 (2026-06-01)
- `FindImageByRect` 通过坐标容差匹配目标占位图片
- `ReplaceImage` 通过 pdfcpu.UpdateImagesByObjNr 替换图片流
- 含 contain 缩放适配逻辑（居中留白）
- 坐标匹配测试通过（V1 匹配 Im10）

---

## 阶段四：边框 + 上市备注

### [ ] 4.1 isNew 边框
- 在替换后的图片上叠加边框
- 边框颜色 = `brand-config.borderColor`（含 opacity）
- 边框宽度 = `brand-config.borderSize`
- `isNew=false 或无该字段` → 无边框

### [ ] 4.2 上市备注文字
- 在图片上方居中显示文字
- 颜色 = `brand-config.descColor`
- 自适应字号（宽度不超过灯片宽度）
- 可选字段，缺失或空字符串不处理

---

## 阶段五：并行处理

### [ ] 5.1 Worker 池实现
- 在 `parallel/worker.go` 中实现
- `errgroup` 管理并发
- channel 传递素材匹配结果到主 goroutine
- 主 goroutine 串行执行 PDF 操作（避免并发写入）

### [ ] 5.2 集成并行到主流程
- 将阶段三/四逻辑接入 worker
- 主 goroutine 收集所有结果后串行执行实际 PDF 替换

---

## 阶段六：表格 + 页面拓展

### [ ] 6.1 列宽计算
- 解析 `table-config`
- 固定宽度列 + 剩余宽度均分给无宽度列
- 总宽度适配页面内容区域

### [ ] 6.2 表格渲染
- 表头：加粗、大字号、自定义背景色
- 表体：字号 `fontSize`、颜色 `brand-config.color`
- 数据源：`isNew=true` 的灯位，按 `table-config.key` 取值

### [ ] 6.3 页面高度拓展
- 计算表格所需高度
- 修改 `/MediaBox` 增加页面高度，宽度不变
- 表格紧贴原内容底部

---

## 阶段七：CLI 完善 + 集成测试

### [ ] 7.1 CLI 参数完善
- 支持 `-o` 输出路径
- 默认输出名 = JSON 文件名更换后缀
- `-cpu` 并发数控制（可选）

### [ ] 7.2 完整流程集成
- 串联阶段一至六的所有模块
- 用 `1.json`（含实际素材和模板）跑通全流程
- 输出 PDF 人工验证：图片替换、边框、表格、页面高度

### [ ] 7.3 边缘案例测试
- 素材缺失（跳过）
- 全 isNew=false（无边框、无表格）
- 上市备注超长（缩字号）
- opacity=0 效果
- 重复 num.str（后者覆盖前者）

---

## 阶段八：收尾

### [ ] 8.1 错误处理 + 日志
- 统一错误报告格式
- 日志输出到 stderr，不污染 stdout

### [ ] 8.2 提交代码
- git init → commit → push
- 提交信息：`feat: 完成 pdf-replace 全部功能`