# pdf-replace — 项目技术规格书

> 本文档供任何 AI 智能体或开发者使用，包含完整的需求、数据结构、算法、边缘案例，足够直接开始编码。

---

## 1. 项目概述

**功能:** 根据一个 JSON 配置文件，将 YSL 店铺灯位画面素材替换到平面图 PDF 模板中，并在底部添加 `isNew` 灯位的表格。

**核心流程:**
1. 读取一个 JSON 文件（如 `1.json`）
2. 对每个灯位并行处理：从素材 PDF 中选出最合适的图片
3. 替换平面图 PDF 模板中对应位置的占位图片
4. `isNew=true` 的替换图片添加边框
5. 有 `上市备注` 字段时在灯片上方叠加文字
6. 在 PDF 底部添加 `isNew=true` 灯位的表格（拓展页面高度）
7. 输出一个新的 PDF

**语言:** Go

**项目类型:** 独立 CLI 工具

---

## 2. CLI 接口

```
pdf-replace [input.json] [-o output.pdf]
pdf-replace /path/to/1.json
pdf-replace /path/to/1.json -o /path/to/output.pdf
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `input.json` | 是 | JSON 配置文件路径 |
| `-o` | 否 | 输出 PDF 路径。默认：输入文件同目录下，文件名同 JSON 名（如 `1.json` → `1.pdf`） |

---

## 3. JSON 数据结构详解

### 3.1 shopName

```json
"shopName": "上海七宝领展广场BD精品店"
```
店铺名称，用于日志/调试输出。

### 3.2 table-config — 表格列定义

```json
"table-config": [
  {"label": "柜台名称", "key": "柜台名称", "width": 190, "align": "left"},
  {"label": "灯位编号", "key": "灯位编号", "width": 50,  "align": "center"},
  {"label": "灯位位置", "key": "灯位位置", "width": 70,  "align": "center"},
  {"label": "材质",       "key": "材质",       "width": 70,  "align": "left"},
  {"label": "可见宽(Mm)","key": "可见宽",    "width": 60,  "align": "left"},
  {"label": "可见长(Mm)","key": "可见长",    "width": 60,  "align": "left"},
  {"label": "灯位备注", "key": "灯位备注",                     "align": "left"},
  {"label": "画面内容", "key": "画面内容",                     "align": "left"}
]
```

- 有 `width` 的列使用固定宽度
- 无 `width` 的列（灯位备注、画面内容）平均分配剩余宽度
- `align` 控制文字对齐方式

### 3.3 brand-config — 品牌样式配置

```json
"brand-config": {
  "borderColor": { "red": 1, "green": 0, "blue": 0, "opacity": 1, "rgb": "rgba(255, 0, 0, 1)" },
  "borderSize": 2,
  "color":       { "red": 1, "green": 0, "blue": 0, "opacity": 1, "rgb": "rgba(255, 0, 0, 1)" },
  "descColor":   { "red": 1, "green": 0, "blue": 0, "opacity": 1, "rgb": "rgba(255, 0, 0, 1)" },
  "descFontSize": 8,
  "fontData": "",
  "fontFamily": "",
  "fontSize": 8
}
```

| 字段 | 用途 |
|------|------|
| `borderColor` | isNew 图片替换后的边框颜色 |
| `borderSize` | 边框粗细（pt） |
| `color` | 表格体文字颜色 |
| `descColor` | 上市备注文字颜色 |
| `descFontSize` | 备注字号 |
| `fontSize` | 表格体字号 |
| `fontData/fontFamily` | 字体 — 目前为空，需要程序内置中文字体（如 Noto Sans SC 或 Source Han Sans） |

**颜色处理** — 每个颜色有 `red/green/blue`（0-1 浮点数）+ `opacity`（0-1）：
- `opacity=1` → 不透明
- `opacity=0` → 完全透明（等于没有）
- 中间值需要在 PDF 中设置对应的 alpha 透明度

### 3.4 excel-data — 灯位数据

```json
"excel-data": {
  "1": {
    "柜台编号": "Y01142116",
    "渠道": "BD",
    "区域": "华东区",
    "城市": "上海",
    "柜台名称": "上海七宝领展广场BD精品店",
    "灯位编号": "1",
    "灯位备注": "SKINCARE-人脸-2605",
    "大类": "SC",
    "灯位位置": "SKINCAREBAY-有假样",
    "材质": "卡布灯箱",
    "可见宽": 1260,
    "可见长": 1282,
    "上市灯片": "Y",         // 可选，等价于 isNew
    "比例(W/H)": 0.982839313572543,
    "比例(H/W)": 1.01746031746032,
    "冲外/冲内": "冲内",
    "画面内容": "全模特",
    "画面位置": "背柜墙上",
    "isNew": true,
    "上市备注": "xxx"         // 可选，有内容时在灯片上方叠加显示
  },
  ...
}
```

**关键字段映射:**

| JSON 字段 | 用途 |
|-----------|------|
| `灯位编号` | 唯一标识，连接 db-data 中的 num.str |
| `可见宽` / `可见长` | 计算目标宽高比 `targetRatio = 可见宽/可见长` |
| `灯位备注` | 映射到 file-data 的 key（素材 PDF 名称） |
| `isNew` | 决定是否为该灯位加边框 + 是否加入底部表格 |
| `画面内容` | 表格数据列 |
| `上市备注` | 可选。存在且非空时，在替换图片上方居中显示，字号自适应不超过灯片宽度，颜色用 descColor |

**匹配规则** — `灯位备注` → `file-data` key 是一个**模糊匹配**：
- 去除空格、全半角转换后匹配
- 如 `"SKINCARE-人脸-2605"` 匹配 `"SKIN CARE-人脸-2605.pdf"`
- 策略：移除所有空格后比较字符串，或使用 Levenshtein 距离 < 3

### 3.5 db-data — 模板关联数据

```json
"db-data": {
  "id": 313,
  "品牌": "Ysl",
  "店铺名称": "上海七宝领展广场BD精品店",
  "lamp": [
    {
      "文件": "guide/Ysl/上海七宝领展广场BD精品店_164.pdf",
      "宽度": "1010.268",
      "高度": "714.3312",
      "缩放": "1.2",
      "编号列表": "[
        {
          \"image\": {
            \"id\": \"113-img_p163_4\",
            \"pageWidth\": 841.89,
            \"pageHeight\": 595.276,
            \"x\": 736,
            \"y\": 213,
            \"width\": 33,
            \"height\": 61,
            \"rotate\": 0
          },
          \"num\": {
            \"str\": \"V1\",
            \"x\": 714.6152,
            \"y\": 226.5929
          },
          \"nums\": [...],
          \"distance\": 16.6227
        },
        ...
      ]"
    }
  ]
}
```

**关键说明：**

- `"编号列表"` 是一个 **JSON 字符串**，需要先反序列化
- `lamp` 数组通常只有一项
- `image` 是 PDF 模板中占位图片的坐标和尺寸（单位：PDF 点，1pt=1/72inch）
- `num.str` 是灯位编号，与 `excel-data` 的 `灯位编号` 匹配
- `"文件"` 是相对于 **base 路径** 的 PDF 路径
  - base: `/Users/Yau/Library/Application Support/com.lorealchina.mplus`
  - 完整路径：`/Users/Yau/Library/Application Support/com.lorealchina.mplus/guide/Ysl/上海七宝领展广场BD精品店_164.pdf`
- **hasMatch 字段忽略**，所有条目平等处理
- 模板 PDF 永远是 **单页**（page 信息在此数据集里不重要）

**图片定位方法** — 在 PDF 中通过 `(pageWidth, pageHeight, x, y, width, height)` 找到一个矩形区域内的嵌入图片对象：
1. 遍历页面的 XObject/Form 资源
2. 对每个图片，检查其变换矩阵的平移（Tx, Ty）和缩放（Sx, Sy）
3. 与目标矩形匹配（考虑坐标系的转换：PDF 原点在左下角，y 轴向上）
4. 注意 `rotate` 不为 0 时的旋转处理

### 3.6 file-data — 素材文件数据

```json
"file-data": {
  "OR ROUGE-全产品-0801.pdf": {
    "path": "/Users/Yau/Downloads/1.loreal/.../OR ROUGE-全产品-0801.pdf",
    "totalPages": null,
    "lazy": true,
    "pages": [
      {
        "p": 1,
        "images": [
          {
            "ext": "png",
            "height": 307,
            "index": 5,
            "page": 1,
            "path": "/Users/yau/Library/Application Support/.../page_001_image_005.png",
            "source": "render-crop",
            "width": 367
          },
          ...
        ]
      }
    ]
  }
}
```

- **key**: 素材 PDF 文件名（不含路径）
- `path`: 原始素材 PDF 路径（但不需要用这个提取图片）
- `pages[].images[]`: **已经提取好的 PNG 图片文件列表**
  - `path`: 可直接使用的绝对路径
  - `width` / `height`: 图片原始尺寸（像素）
  - `source`: `"render-crop"` 或 `"direct-fast"`，仅用于日志，处理时一视同仁
- **所有页的所有图片**都参与匹配

---

## 4. 核心算法

### 4.1 总流程

```
for each entry in db-data.lamp[0].编号列表  (并行 goroutine)
  |→ 取 num.str（灯位编号）
  |→ 在 excel-data 中按 灯位编号 找到对应灯位
  |   |→ 目标宽高比 = 可见宽 / 可见长
  |   |→ 目标方向 = targetRatio >= 1 ? "横图" : "竖图"
  |   |→ 通过 灯位备注 匹配 file-data 中的素材 PDF
  |   |   |→ 素材只有1张图片？直接使用，跳过匹配
  |   |   |→ 素材有多张图片？执行方向 + 宽高比匹配
  |   |→ 没找到素材？跳过本次替换（保留原占位图片）
  |→ 有匹配素材？
  |   |→ 替换 PDF 模板中 image 坐标处的图片
  |   |→ 是否 isNew=true？加边框（borderColor + borderSize）
  |   |→ 是否 上市备注 非空？在图片上方居中显示文字（descColor）
  |
  ↓ 所有并行任务完成后
  → 筛选 excel-data 中 isNew=true 的灯位
  → 在 PDF 底部添加表格（拓展页面高度）
  → 输出最终 PDF
```

### 4.2 图片方向匹配规则

**目标方向判定** — 根据 `可见宽/可见长`：

| 目标宽高比 | 方向 |
|-----------|------|
| `ratio > 1` | 横图（宽 > 高） |
| `ratio < 1` | 竖图（高 > 宽） |
| `ratio == 1` | 正方形（特殊处理） |

**素材方向判定** — 对素材 PDF 中每张图片计算 `imgRatio = width / height`

**匹配策略：**

```
if 素材图片数量 == 1:
    直接使用该图片
else:
    if 目标方向 == 横图 (targetRatio > 1):
        只从素材中筛选 imgRatio > 1 的图片
        选 |imgRatio - targetRatio| 最小的一张
    elif 目标方向 == 竖图 (targetRatio < 1):
        只从素材中筛选 imgRatio < 1 的图片
        选 |imgRatio - targetRatio| 最小的一张
    elif 目标方向 == 正方形 (targetRatio == 1):
        寻找素材中 imgRatio == 1 的图片（浮点误差容忍 ≤ 0.01）
        找到则直接使用
        找不到则：
            横图候选 = 素材中 imgRatio > 1，选 |imgRatio - 1| 最小
            竖图候选 = 素材中 imgRatio < 1，选 |imgRatio - 1| 最小
            比较二者与 1 的差距，更接近的那个胜出
```

**边缘情况处理：**
- 素材全部横图但目标是竖图 → 取最接近竖图比例的横图（即宽高比最小的）
- 素材全部竖图但目标是横图 → 同理
- 完全没有同方向图片 → 选整体宽高比最接近的那张
- 素材匹配失败（无可用的图片）→ 跳过，保留原占位图

### 4.3 图片替换适配策略

**contain 模式 — 保持比例，最长边填满占位框：**

```
占位框尺寸: targetW, targetH（来自 image.width, image.height）
素材图片尺寸: imgW, imgH

scale = min(targetW / imgW, targetH / imgH)
newW = imgW * scale
newH = imgH * scale

偏移量（居中）:
offsetX = (targetW - newW) / 2
offsetY = (targetH - newH) / 2

最终素材图片在占位框中的矩形：(offsetX, offsetY, newW, newH)
```

剩余区域留白（透明或白色，取决于 PDF 背景）。

### 4.4 isNew 边框

- 仅 `isNew = true` 的灯位，替换后的图片需要加边框
- 边框使用 `brand-config.borderColor`（含 opacity）+ `borderSize`
- 边框画在占位框边缘（非图片边缘），即 `(0, 0, targetW, targetH)` 矩形
- `isNew = false` 或缺失 → 无边框

### 4.5 上市备注文字

- 字段名：`"上市备注"`
- 仅当前灯位有此字段且值非空时处理
- 文字叠加在图片**上方**（图片顶部以上）
- 文字颜色：`brand-config.descColor`（含 opacity）
- 字号：以 `brand-config.descFontSize` 为初始值，如果文字宽度超过灯片宽度则逐步缩小（缩小步长 0.5pt），直到文字宽度 ≤ 灯片宽度
- 对齐：水平居中于灯片
- 垂直位置：图片上边缘以上留少量间距（如 2pt）

### 4.6 表格生成

**数据源：** `excel-data` 中 `isNew = true` 的所有灯位

**列定义：** `table-config`

**列宽算法：**
```
固定宽度列总宽 = sum(config.width for config where config.width exists)
页面内容宽度 = PDF页面宽度 - 左右边距（如各30pt）
剩余宽度 = 页面内容宽度 - 固定宽度列总宽
无宽度列数 = count(config where config.width missing)
每列剩余宽 = 剩余宽度 / 无宽度列数
```

**表头样式：**
- 背景色：开发者自定（建议使用 brand-config 的品牌色变体，如深红色背景白色文字，或浅红色背景黑色文字）
- 文字：**加粗**
- 字号：比表体大（建议 10-11pt）
- 字体：需要中文字体支持

**表体样式：**
- 文字颜色：`brand-config.color`
- 字号：`brand-config.fontSize`（默认 8）
- 文字对齐：按 `table-config` 中每列的 `align`
- 字体：需要中文字体支持

**表格位置：**
- 紧贴页面原有内容的底部
- **页面需要拓展高度**（宽度不变）以容纳表格
- 表格上方保留行间距（如 10-15pt）

---

## 5. 并行处理设计

### 5.1 并行粒度

- 每个 `db-data.lamp[0].编号列表` 条目（即每个灯位）一个 goroutine
- 使用 `errgroup` 或 `sync.WaitGroup` + `sync.Mutex` 管理

### 5.2 共享资源保护

- PDF 模板文件的图片替换不能并行（多 goroutine 同时修改同一个 PDF 对象不安全）
- **推荐方案:** 每个 goroutine 处理素材匹配后，将替换信息发回主 goroutine，由主 goroutine 串行执行 PDF 替换操作

```
主 goroutine:
  读取JSON, 解析数据, 打开模板PDF
  启动 N 个 worker goroutine

worker goroutine (每个灯位):
  1. 匹配 excel-data 灯位
  2. 匹配 file-data 素材
  3. 执行方向+宽高比匹配算法
  4. 选中最终素材图片
  5. 结果发送到 channel: {image坐标, 素材图片路径, isNew, 上市备注}

主 goroutine:
  for 每个 channel 结果:
    串行替换 PDF 中的图片
    加边框（如需）
    加上市备注（如需）

  所有替换完成后:
    添加 isNew 表格
    输出 PDF
```

### 5.3 并发数控制

- 使用 `errgroup` 限制并发数，建议默认 `GOMAXPROCS` 或通过 `-cpu` 参数控制（参考 pdf-tool 的 `-cpu` 参数设计，与其保持一致）
- 界面上：避免过多 goroutine 同时读文件导致 I/O 瓶颈

---

## 6. PDF 技术方案

### 6.1 Go PDF 库选择

推荐（按优先级）：

| 库 | 支持情况 | 说明 |
|----|---------|------|
| **pdfcpu** | 读写 + 修改 | 开源、活跃、支持图片替换（通过修改 stream），但不直接支持"坐标找图" |
| **unipdf** (免费版) | 完整 PDF 能力 | 最好用但商用需授权，免费版有页数限制 |
| **gopdf** | 生成 | 只能创建新 PDF，不能修改已有 PDF |
| **gofpdf** | 生成 | 同上 |

**推荐方案：混合使用 pdfcpu + gopdf/原生 PDF 操作**
- 使用 `pdfcpu` 读取模板 PDF、遍历 XObject、替换图片流
- 使用 `pdfcpu` 的页面尺寸修改功能拓展页面高度
- 表格部分可用 `gopdf` 生成一个新页面层合并到底部

**或者：全手动 PDF 操作（最可控但较复杂）**
- 用 Go 标准库 + 原生 PDF 语法解析
- 直接操作 PDF 对象流替换图片数据
- 适合对 PDF 结构有深入理解的情况

### 6.2 图片替换方案

在 PDF 中找到目标图片对象的方法：
1. 解析页面 `/Contents` 流中的操作符
2. 查找 `Do` 操作（图片绘制指令）
3. 获取其变换矩阵 `[a b c d e f]`，其中 `(e, f)` 是平移（x, y），`(a, d)` 是缩放
4. 与 `image` 中的 `(x, y, width, height)` 匹配（考虑 PDF 坐标系统：原点左下角，y 向上）
5. 匹配到的图片对象 → 替换其 `/Subtype /Image` 的流数据为新 PNG

### 6.3 页面高度拓展

- 当前模板 PDF 页面尺寸（如 841.89×595.276 pt = A4 横向）
- 计算表格所需高度 = 表头高度 + sum(每行数据高度) + 边距
- 新页面高度 = 原高度 + 表格所需高度 + 额外间距
- 使用 pdfcpu 的 `api.SetPageDimensions` 或直接修改 `/MediaBox`

### 6.4 中文字体

由于表格和上市备注需要显示中文，需要嵌入中文字体：
- 推荐使用 **NotoSansSC-Regular.otf** 或 **SourceHanSansSC-Regular.otf**
- 字体文件可以：a) 硬编码到二进制中（使用 `embed`），b) 通过参数指定路径
- 默认建议使用 `embed` 嵌入一个精简版中文字体 ~1-2MB

---

## 7. 边缘案例汇总

| 场景 | 处理方式 |
|------|---------|
| 灯位编号在 excel-data 中找不到 | 跳过该条目的替换，记录 warn 日志 |
| 灯位备注匹配不到 file-data | 跳过替换，保留原占位图 |
| file-data 匹配成功但素材 PDF 的 pages 为空 | 跳过替换 |
| 素材图片全部同一方向，目标方向无匹配 | 选宽高比最接近的那张 |
| 素材图片宽高比为0或负数 | 跳过该图片 |
| isNew 字段缺失 | 视为 false（不加边框，不入表格） |
| 上市备注字段缺失或为空字符串 | 不添加文字 |
| brand-config 某些字段缺失 | 使用默认值（红、8pt、不透明） |
| JSON 中 编号列表 不是有效 JSON | 报错退出 |
| 模板 PDF 不存在或无权限 | 报错退出 |
| 输出路径无法写入 | 报错退出 |
| 颜色 opacity=0 | 完全透明，等于不处理该元素 |
| 编号列表中有重复 num.str | 各自独立处理，后处理的覆盖前一个 |

---

## 8. 项目文件结构

```
pdf-replace/
├── main.go                 — CLI 入口，解析参数
├── cmd/
│   └── replace.go          — 主逻辑编排
├── config/
│   └── json.go             — JSON 解析和数据模型
├── matcher/
│   ├── matcher.go          — 素材匹配算法（方向+宽高比）
│   └── matcher_test.go     — 匹配算法单元测试
├── pdf/
│   ├── template.go         — 模板 PDF 操作（打开、替换图片、修改尺寸）
│   ├── table.go            — 表格生成
│   └── text.go             — 文字叠加（上市备注）
├── model/
│   └── types.go            — 所有数据结构的 Go 类型定义
├── parallel/
│   └── worker.go           — 并行工作池管理
├── assets/
│   └── NotoSansSC-Regular.otf  — 嵌入式字体文件
└── go.mod
```

---

## 9. 测试场景

### 9.1 单元测试

- 图片方向匹配算法（横图/竖图/正方形/单图直接使用）
- 宽高比最近距离计算
- 列宽分配计算
- 颜色 opacity 处理

### 9.2 集成测试

- 用提供的 `1.json` + 现有的素材/模板 PDF 跑完整流程
- 验证输出 PDF 中图片被正确替换
- 验证 isNew 边框存在
- 验证表格内容与 isNew 灯位一致
- 验证页面高度已拓展
- 验证上市备注文字位置和样式

### 9.3 边缘测试

- 素材无可匹配图片（保留原图）
- 所有灯位 isNew=false（无表格、无边框）
- 上市备注内容超长（自动缩字号的验证）
- opacity=0 的效果

---

## 10. 实现优先级建议

1. **数据类型 + JSON 解析** — 先定义所有 Go 结构体，解析 `1.json` 验证数据模型
2. **素材匹配算法** — 方向 + 宽高比匹配逻辑（可脱离 PDF 单独测试）
3. **PDF 模板中的图片定位** — 在 PDF 中通过坐标找到图片对象
4. **图片替换 + 适配策略** — 替换图片 + contain 缩放
5. **isNew 边框 + 上市备注** — 叠加边框和文字
6. **并行改造** — 将单灯位处理变为并行 worker
7. **底部表格 + 页面拓展** — 生成表格并拓展页面
8. **CLI 参数 + 输出路径** — 完善命令行接口