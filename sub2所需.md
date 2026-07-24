吧文件传到另外一个服务器
---传文件
rsync -avzP /www/backup/database/sub2api_2026-07-21_23-31-37_pgsql_data.sql.gz root@170.178.174.119:/www/backup/database/

rsync -avzP /www/backup/database/pgsql/sub2api/sub2api_2026-07-24_21-24-32_pgsql_data.sql.gz root@170.178.174.119:/www/backup/database/


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



## PGSQL 相关
#### 1. 安装 postgresql 全套（服务端 + 客户端）

```
apt update
apt install postgresql postgresql-client -y
```

安装完成后，`psql`命令就会被注册到系统环境变量。

#### 2. 验证服务运行状态

```
systemctl status postgresql
```

若未启动，执行启动、开机自启：

```
systemctl start postgresql
systemctl enable postgresql
```

#### 3. 本机登录 PostgreSQL（两种常用方式）

##### 方式 1：系统用户免密登录（推荐）

```
sudo -u postgres psql
```

成功后会进入`postgres=#`数据库交互终端。