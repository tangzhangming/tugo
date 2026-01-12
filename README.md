# tugo

go build -o tugo.exe ./cmd/tugo

# 运行单个文件
tugo run examples\hello.tugo

# 运行整个项目目录
tugo run examples\import_demo

# 详细输出
tugo run -v examples\hello.tugo


# 默认输出到 output 目录
tugo build examples\hello.tugo

# 指定输出目录
tugo build -o mydir examples\hello.tugo

# 构建整个项目
tugo build -o dist examples\import_demo

# 详细输出
tugo build -v examples\hello.tugo
