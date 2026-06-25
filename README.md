# novel

小说网文下载工具，支持从多个书源搜索、获取章节，导出 **TXT** 和 **EPUB** 格式。

## 使用方法

### 方式一：下载二进制包

点击 [Release](https://github.com/rjshuang/novel/releases) 下载对应平台的二进制包，解压后运行。

### 方式二：自行编译

```bash
git clone https://github.com/rjshuang/novel.git
cd novel
go build -o novel .
```

### 命令行参数

```bash
novel -o ./output
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-o` | `download/` | 输出目录 |

## 执行流程

```
选择书源 → 搜索书籍 → 选择书籍 → 获取章节 → 选择章节范围 → 下载（TXT + EPUB）
```

## 下载格式

- **TXT** — 纯文本格式，标题+正文
- **EPUB** — 电子书格式，带封面、CSS 样式、章节结构

## 内置书源

目前内置 5 个免登录书源（可根据 `rules.json` 规则扩展）：

| 书源 | 网站 |
|------|------|
| 101看书网 | [101kanshu.net](https://www.101kanshu.net) |
| 22笔趣阁 | [22biqu.com](https://www.22biqu.com) |
| 笔趣阁 | [beqege.net](https://www.beqege.net) |
| 落霞小说 | [luoxia123.com](https://www.luoxia123.com) |
| 速读谷 | [sudugu.org](https://www.sudugu.org) |

## 使用示例

```
书源ID  网站
0       101看书网
1       22笔趣阁
2       笔趣阁
3       落霞小说
4       速读谷

>> 选择书源: 2
>> 输入书名或作者名: 凡人修仙传
索引  书名       作者    更新时间        最新章节
0     凡人修仙传  忘语    2024-01-01      第一千章 ...

>> 输入开始章节序号（输入 -1 从头开始）: -1
>> 输入结束章节序号（输入 -1 到末尾）: -1

[128/256] 50.0%  第一千零二十八章 灵界之战

下载完成，失败章节数: 0
TXT 文件保存成功: download/凡人修仙传.txt
EPUB 文件保存成功: download/凡人修仙传.epub
```

## 项目结构

```
novel/
├── main.go              # 入口，CLI 交互 + 下载调度 + TXT/EPUB 导出
├── main_test.go         # EPUB 生成测试
├── rules.json           # 书源规则配置（XPath 爬虫规则）
├── common/common.go     # 通用工具（URI格式化、分批切片、去重）
├── search/search.go     # 搜索与爬取核心（搜索、章节列表、内容爬取）
├── htmlquery/query.go   # 简易 XPath 解析引擎
├── http_util/http_util.go # HTTP 请求工具（支持代理）
└── static/cover.css     # EPUB 封面样式
```

## 注意事项

- 下载失败的章节会记录索引和错误信息，不影响其他章节下载
- 章节选择时输入 `-1` 表示"从头开始"或"到末尾"
