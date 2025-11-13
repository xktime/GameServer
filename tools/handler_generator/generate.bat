@echo off
echo 正在生成C2S消息handler...
go run .  -proto D:\mine\GameServer\common\msg\proto -output D:\mine\GameServer\common\msg\message -modules ../../modules -ingoreFile rpc.proto
echo 生成完成！
pause 