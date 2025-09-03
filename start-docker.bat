@echo off
echo ========================================
echo CDK Server Docker 启动脚本
echo ========================================

echo.
echo 正在检查Docker状态...
docker --version >nul 2>&1
if %errorlevel% neq 0 (
    echo 错误: Docker未安装或未启动
    echo 请确保Docker Desktop正在运行
    pause
    exit /b 1
)

echo Docker状态正常

echo.
echo 正在停止并删除旧容器...
docker stop gameserver >nul 2>&1
docker rm gameserver >nul 2>&1

echo.
echo 正在构建Docker镜像...
docker build -t gameserver:latest .

if %errorlevel% neq 0 (
    echo 错误: Docker镜像构建失败
    pause
    exit /b 1
)

echo.
echo Docker镜像构建成功！

echo.
echo 正在启动容器...
docker run -p 3653:3653 -p 3563:3563 --network my-network --name gameserver gameserver:latest

if %errorlevel% neq 0 (
    echo 错误: 容器启动失败
    pause
    exit /b 1
)

echo.
echo 容器启动成功！
echo.
echo 容器信息:
docker ps --filter name=gameserver

echo.
echo 按任意键查看容器日志...
pause >nul

echo.
echo 容器日志:
docker logs gameserver

echo.
echo 按任意键退出...
pause >nul
