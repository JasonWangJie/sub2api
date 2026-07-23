吧文件传到另外一个服务器
---传文件
rsync -avzP /www/backup/database/sub2api_2026-07-21_23-31-37_pgsql_data.sql.gz root@170.178.174.119:/www/backup/database/


curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/install.sh | sudo bash
# 1. 启动服务
sudo systemctl start sub2api
sudo systemctl stop sub2api

# 2. 设置开机自启
sudo systemctl enable sub2api

# 3. 在浏览器中打开设置向导
# http://你的服务器IP:8080


# 查看状态
sudo systemctl status sub2api

# 查看日志
sudo journalctl -u sub2api -f

# 重启服务
sudo systemctl restart sub2api

# 卸载
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/install.sh | sudo bash -s -- uninstall -y