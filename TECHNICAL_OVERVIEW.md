# pdf-replace — 技术选型与路线总结

> 店铺灯位画面自动替换工具：将 YSL 店铺的素材图片批量替换到平面图 PDF 模板中，
> 并追加 isNew 表格、独立矢量边框、上市备注文字。
>
> 最终指标：10 张图片替换 → **1.8MB 输出 / 0.097s 完成**

---

## 一、业务逻辑概述

```
输入 (1.json)
  ├─ db-data         → 模板 PDF 路径 + 灯位编号列表（含占位图坐标）
  ├─ excel-data      → 灯位元数据（备注、尺寸、是否新灯位）
  ├─ file-data       → 素材 PDF 中的图片清单（路径、方向、尺寸）
  └─ brand-config    → 品牌配色方案（边框颜色、字体大小）
       │
       ▼
   替换流程
       │
       ▼
输出 (output.pdf)
  ├─ 模板 PDF（原始平面图）
  ├─ 替换后的灯位图片（素材 → 占位图）
  ├─ isNew 灯位的独立矢量边框
  ├─ 上市备注文字（在图片正上方）
  └─ 底部 isNew 表格（品牌红色系）
```

---

## 二、项目结构

```
pdf-replace/
├── main.go              入口：参数解析（-o, -cpu）
├── cmd/
│   └── replace.go       主流程编排：Run() + processImageDirect() + renderTable()
├── config/
│   └── json.go          JSON 加载与校验
├── model/
│   └── types.go         全部数据结构定义 + 自定义 JSON 解码
├── matcher/
│   ├── matcher.go       图片匹配：按方向/宽高比/尺寸选最优
│   └── matcher_test.go
├── pdf/
│   ├── template.go      模板操作：打开、解析图片位置、替换StreamDict、矢量边框
│   ├── draw.go          图片处理：缩放、文字叠加、边框渲染
│   ├── table.go         原生 PDF 表格生成（gopdf）
│   ├── extend.go        页面高度扩展
│   ├── template_test.go
│   └── template.go 参考数据
└── assets/
    └── hyzdx.ttf        中文字体
```

## 三、核心依赖选型

| 库 | 版本 | 用途 | 选型理由 |
|---|---|---|---|
| **pdfcpu** | v0.12.1 | PDF 底层操作：打开模板、解析 xref table、查找 XObject、替换 Stream | Go 生态最成熟的 PDF 库，原生支持 pdf 流操作，不依赖外部 CLI |
| **gopdf** | v0.36.1 | 生成 isNew 表格 PDF | 原生支持中文排版、字体嵌入、表格定位 |
| **golang.org/x/image** | — | 图片缩放用的 `draw.BiLinear` 等 | Go 官方扩展，纯 Go 实现，跨平台兼容 |
| Go stdlib `image/jpeg` | — | JPEG 编码（Filter: DCTDecode） | 高度优化的 DCT+Huffman 编码，比 Flate 快 3-4x |
| Go stdlib `image/png` | — | 解码素材 PNG 图片 | Go 原生支持，自动识别格式 |

### 为何不用 GhostScript / ImageMagick？
- **GhostScript**：CLI 调用有 50-100ms 启动开销，多图场景累积明显；且文件输出需经 PS→PDF 转换，控制精度不够
- **ImageMagick**：同样 CLI 启动开销 + 临时文件 IO，精度不如直接 `image.Image` → `StreamDict`
- **结论**：纯 Go 内联处理 = 最优（零进程开销，内存直接像素操作）

---

## 四、核心流水线详解

### Step 1 — 准备阶段（串行，~几毫秒）
```
JSON 配置 → 解析 db-data → 获取模板 PDF 路径 → pdfcpu.OpenTemplate()
                                                        ↓
                              遍历编号列表，对每个灯位：
                                → 取占位图坐标 (X,Y,W,H)
                                → FindImageByRect() 匹配 PDF 中的 XObject
                                → ExcelData 匹配 lampItem
                                → FileData 匹配素材图片
                                → SelectBestImage() 按方向/宽高比选最优图
```

### Step 2 — 并行图片处理（Worker Pool，~80ms）
```
Worker Pool（默认 4 goroutine，通过 -cpu N 控制）

每个 Worker 循环：
  1. Open + image.Decode(source.png)
  2. ScaleImageContain(img, targetW, targetH)  ← 缩放到灯位尺寸
  3. DrawTextOnTop(img, launchNote)            ← 叠加上市备注
  4. ImageToStreamDictJPEG(img, 85)            ← JPEG 编码为 StreamDict
  5. 通过 channel 发送 processed{objNr, *sd}
```

### Step 3 — 串行替换（~5ms）
```
遍历 results channel：
  → tmpl.ReplaceStreamDict(objNr, sd)   ← 仅 xref 表交换，无像素操作
  → 记录 isNew 灯位信息（边框 + 表格用）
```

### Step 4 — 独立矢量边框（~2ms）
```
对每个 isNew 灯位：
  → FindImageByObjNr(objNr) → 获取显示位置
  → DrawRectBorder()         → 追加 PDF 矢量描边指令
      q <r> <g> <b> RG <w> w <x> <y> <w> <h> re S Q
```

### Step 5 — 表格生成与注入（~10ms）
```
WriteTableToPDF()  → gopdf 生成原生 PDF 表格（含中文）
InjectTableContent() → pdfcpu 合并表格页 → 追加到模板页面底部
```

---

## 五、性能优化旅程（三阶段）

### 原始版本瓶颈分析
```
流程: Decode PNG → Encode PNG(bytes) → CreateImageStreamDict(再decode) → Flate压缩
                                                                   ↑↑
                                                    两次解码+一次多余编码=核心浪费
```

### 第 1 轮：跳过中间 PNG 编解码
- **改动**：`processImage()` 返回 `image.Image`，新增 `ImageToStreamDict()` 直接 Flate 压缩 raw RGB
- **结果**：2.52s → **0.61s**（4.1x）

### 第 2 轮：缩放到灯位显示尺寸
- **问题**：素材图片 2000-4000px，灯位仅 200-400pt（72DPI = 像素），像素浪费 10-40x
- **改动**：在 decode 后立即 `ScaleImageContain(img, targetW, targetH)`，使用最近邻插值
- **结果**：0.61s → **0.33s**，32MB → **7.2MB**（速度 + 1.8x，体积 - 4.4x）

### 第 3 轮：JPEG 替代 Flate
- **原理**：Flate（Deflate）是无损压缩，对照片类图片压缩比只有 2-4x；JPEG（DCT+Huffman）是高度优化的有损编码，压缩比 10-20x，编码速度更快
- **改动**：新增 `ImageToStreamDictJPEG(img, quality)`，Filter 设为 `DCTDecode`
- **结果**：0.33s → **0.097s**，7.2MB → **1.8MB**（速度 + 3.4x，体积 - 4x）

### 完整路线图

| 阶段 | 文件大小 | 速度 | 加速比（累积） |
|------|---------|------|---------------|
| 原始（PNG→Flate） | 32MB | 2.52s | 1x |
| 跳过 PNG 编解码 | 32MB | 0.61s | 4.1x |
| + 缩放至灯位尺寸 | 7.2MB | 0.33s | 7.6x |
| + JPEG@Q85 | **1.8MB** | **0.097s** | **26x** |

### 优化方法论（可复用）
```
1. 砍像素 → 缩放到实际显示尺寸（效果最大）
2. 换编码 → 照片用 JPEG，矢量/文字用 Flate
3. 并行走 → Worker Pool 并行编解码，串行只做 xref 交换
4. 去中介 → 避免不必要的中间格式转换（PNG→PNG→Flate）
```

---

## 六、关键技术细节

### 6.1 图片匹配策略（matcher.go）

```
素材 PDF 可能包含多页，每页 0-N 张图片。
匹配规则：
  1. 按灯位备注（lampNote）匹配素材 PDF 文件名
  2. 按灯位尺寸比例（宽高比）筛选候选图片
  3. 横图配横图（W/H ≥ 1），竖图配竖图（W/H < 1）
  4. 正方形图片（0.9 ≤ W/H ≤ 1.1）取横竖中最接近尺寸的候选
  5. 候选只有一张则直接使用
```

### 6.2 PDF 图片定位（FindImageByRect）

```
如何找到 PDF 中"灯位 1"对应的图片？
  1. 遍历 PDF 页面 Content Stream，提取 cm 矩阵中的平移量 (x,y)
  2. 匹配最近的 image XObject（使用 cm 矩阵的尺寸）
  3. 容差 tolerance=5pt 应对精确匹配失败

定位数据：ImagePosition{ObjNr, Name, Page, X, Y, W, H}
```

### 6.3 原生 PDF 表格（table.go）

```
使用 gopdf 生成纯 PDF 文本表格（可搜索、可复制），而非图片方式：
  → 设置中文字体（hyzdx.ttf TTFFont）
  → 按列宽自动排版（表头加粗大字号，表体品牌红色系）
  → 写入临时 PDF → pdfcpu 合并注入到模板页面底部
  → 自动扩展页面高度以容纳表格
```

### 6.4 独立矢量边框

```
isNew 灯位的边框不画在图片上，而是追加独立的 PDF 矢量描边指令：
  q <r> <g> <b> RG <lineWidth> w <x> <y> <w> <h> re S Q

优点：
  → 不受图片分辨率影响（始终锐利）
  → 线宽由品牌配置控制（borderSize * 20 = 最终 pt）
  → 可单独删除/修改不影响图片层
```

### 6.5 JPEG 编码 vs Flate 压缩

| 特性 | Flate (Deflate) | JPEG (DCTDecode) |
|------|----------------|------------------|
| 压缩类型 | 无损 | 有损 |
| 照片压缩比 | 2-4x | 10-20x (@Q85) |
| 文本/图形质量 | 完美 | 有边缘模糊 |
| 编码速度 | 慢（LZ77+Huffman） | 快（SIMD DCT） |
| PDF 支持 | 所有阅读器 | 所有阅读器 |
| 适用场景 | 文字、图标、矢量图 | 照片、渐变、实拍图 |

---

## 七、构建与运行

```bash
# 构建
cd /Users/Yau/work/1.Resources/2.AI/pdf-replace
go build -o pdf-replace .

# 运行
./pdf-replace 1.json                          # 自动输出到 1.pdf
./pdf-replace 1.json -o output.pdf            # 指定输出
./pdf-replace 1.json -cpu 8                   # 8 worker 并发
./pdf-replace 1.json -o out.pdf -cpu 10       # 组合参数

# 测试
go test ./...
```

## 八、配置文件结构

输入 JSON (`1.json`) 结构：

```
{
  "shopName": "店铺名称",
  "brand-config": {          ← 品牌配色方案
    "borderSize": 2,
    "borderColor": { "red": 0.8, "green": 0, "blue": 0, "opacity": 1 },
    "descColor":  { ... },  ← 上市备注文字颜色
    "descFontSize": 16,
    ...
  },
  "table-config": [          ← isNew 表格列定义
    { "label": "灯位编号", "key": "灯位编号", "width": 60, "align": "center" },
    ...
  ],
  "excel-data": {            ← 灯位元数据（按编号索引）
    "V1": { "灯位备注": "...", "可见宽": 200, ... },
    ...
  },
  "db-data": {               ← 模板 PDF 数据
    "lamp": [{
      "文件": "guide/Ysl/xxx.pdf",
      "编号列表": "[{...},{...}]"
    }]
  },
  "file-data": {             ← 素材图片清单（按 PDF 名索引）
    "MUSE产品.pdf": { "pages": [{ "images": [{...}] }] },
    ...
  }
}
```

## 九、未来可能的改进方向

1. **自适应编码选择**：图片含大量文字/纯色 → Flate，照片 → JPEG，自动判断
2. **渐进式 JPEG**：支持 Progressive JPEG 以获得更好的网络加载体验
3. **WebP 支持**：PDF 2.0 支持 WebP，压缩比优于 JPEG（需更新 pdfcpu）
4. **空图跳过优化**：当前所有灯位都跑完整流程，无素材灯位可在 prep 阶段提前跳过
5. **软编码 -cpu 阈值**：根据图片数量自动计算最优并发数，减少手动调参
