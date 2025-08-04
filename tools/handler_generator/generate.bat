@echo off
echo 正在生成C2S消息handler...
go run . -proto ../../common/msg/pb -output ../../modules/login/internal/handlers
echo 生成完成！
pause 