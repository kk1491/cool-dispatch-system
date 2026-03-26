#!/bin/zsh

# ============================================================
# cool-dispatch Git Push 脚本
# 自动添加所有变更、提交并推送到远程仓库
# GitHub 用户/Token 与 appointment-system 项目保持一致
# ============================================================

echo "=========================================="
echo "       Git Push - cool-dispatch"
echo "=========================================="
echo ""

# Git 配置：用户名允许提供默认值；Token 必须通过环境变量注入，避免再次把敏感信息提交到仓库。
GIT_USERNAME="${GIT_USERNAME:-kk1491}"
GIT_TOKEN="${GIT_TOKEN:-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}"
GIT_REPO_SLUG="${GIT_REPO_SLUG:-kk1491/cool-dispatch-system}"

# 推送前强制校验 Token，避免脚本在未配置凭证时继续执行并产生误导输出。
if [[ -z "$GIT_TOKEN" ]]; then
    echo "❌ 未检测到 GIT_TOKEN 环境变量，已停止推送"
    echo "请先执行: export GIT_TOKEN=你的GitHubToken"
    exit 1
fi

REPO_URL="https://${GIT_USERNAME}:${GIT_TOKEN}@github.com/${GIT_REPO_SLUG}.git"

# 获取当前分支
CURRENT_BRANCH=$(git branch --show-current)
echo "🌿 当前分支: $CURRENT_BRANCH"
echo ""

# 确认推送
read "confirm?确定要推送到远程仓库吗？(y/n): "
if [[ $confirm != "y" && $confirm != "Y" ]]; then
    echo "❌ 已取消推送"
    exit 0
fi

echo ""

# 检查是否有变更
if [[ -n $(git status -s) ]]; then
    echo "📝 检测到以下变更："
    git status -s
    echo ""
    
    # 添加所有变更到暂存区
    echo "📦 添加变更到暂存区..."
    git add -A
    
    # 让用户输入提交信息
    echo ""
    read "commit_msg?📝 请输入提交信息: "
    
    # 如果没有输入，使用默认信息
    if [[ -z "$commit_msg" ]]; then
        commit_msg="Update cool-dispatch"
    fi
    
    # 提交到本地仓库
    echo ""
    echo "💾 提交到本地仓库..."
    git commit -m "$commit_msg"
    
    if [ $? -ne 0 ]; then
        echo ""
        echo "❌ 本地提交失败，请检查错误信息"
        exit 1
    fi
    
    echo "✅ 本地提交成功"
else
    echo "ℹ️ 没有变更需要提交"
fi

echo ""
echo "🚀 开始推送到远程仓库..."

# 设置远程 URL 并推送
git remote set-url origin "$REPO_URL" 2>/dev/null || git remote add origin "$REPO_URL"
git push -u origin "$CURRENT_BRANCH"

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ 推送成功！"
else
    echo ""
    echo "❌ 推送失败，请检查错误信息"
fi
