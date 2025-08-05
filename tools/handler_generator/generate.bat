@echo off
echo 正在生成C2S消息handler...
go run . -proto ../../common/msg/pb -output ../../common/msg/message/handlers -modules ../../modules
echo 生成完成！
pause 