# ClassDing MVP 协作契约

实现以开发方案为准。本文件冻结 MVP 接口。所有返回值直接返回实体或数组，不包 data。错误 `{code,message}`。除 config、注册登录、device/bind 外均需 Bearer 认证。浏览器访问同源 /api/v1；WS 同源 /ws/teacher 和 /ws/device。

## 固定实体
- User: id, username, display_name, role(user|school_admin|super_admin), org_id
- Class: id,name,grade,owner_id,org_id,volume,role(owner|collaborator),max_teachers,max_devices,teachers_count,devices_count,online_devices,created_at
- Device: id,class_id,name,os,client_version,online,last_seen_at
- Member: user_id,display_name,username,role,joined_at
- Phrase: id,text,sort
- Message: id,kind,text,student_name,signature,style(notice|praise|urgent),display_sec,repeat,chime,play_mode,sender_id,created_at,expires_at,recalled_at,targets: [{class_id,class_name,device_id,device_name,status,seq,delivered_at,played_at}]
- ActivationCode: code,max_teachers,max_devices,expires_at,status(unused|active|revoked|released),class_id,org_id,batch_id,note,redeemed_at
- AuthResponse: {access_token,user}；refresh token 放 HttpOnly Cookie。

## REST
GET config => {speech_enabled:false,registration_enabled:true,version:'0.1.2'}
POST auth/register {username,password,display_name} => AuthResponse
POST auth/login {username,password} => AuthResponse
POST auth/refresh => AuthResponse；POST auth/logout => {ok:true}
GET me => User；PATCH me {display_name} => User；PUT me/password {current_password,new_password} => {ok:true}
GET classes => Class[]；POST classes {name,grade?,activation_code} => Class
PATCH classes/:id {name?,volume?} => Class；DELETE => {ok:true}
GET classes/:id/members => Member[]
POST classes/:id/invites => {code,expires_at}
POST classes/join {code} => Class
DELETE classes/:id/members/:uid => {ok:true}
POST classes/:id/transfer {user_id} => Class；POST classes/:id/leave => {ok:true}
POST classes/:id/bind-code => {code,expires_at}
GET classes/:id/devices => Device[]；PATCH devices/:id {name} => Device；DELETE devices/:id => {ok:true}
POST device/bind {code,name,os,client_version} => {device_token,device,class:{id,name},volume}
GET device/me => {device,class:{id,name},volume}
POST messages {class_ids,kind,text,student_name,signature,style,display_sec,repeat,chime,play_mode,audio_id?} => Message
GET messages?class_id=&cursor= => {items:Message[],next_cursor:string|null}
GET messages/:id => Message；POST messages/:id/recall => Message；POST messages/:id/resend => Message
GET phrases => Phrase[]；POST phrases {text} => Phrase；PATCH phrases/:id {text} => Phrase；DELETE => {ok:true}
POST audio multipart file => {audio_id,url,transcript?}
POST classes/:id/tools/countdown {seconds} => {ok:true}；POST classes/:id/tools/rollcall {names} => {ok:true}；POST classes/:id/tools/stop => {ok:true}
GET client/latest => {version,url,sha256,notes,min_supported}

## 管理后台
GET admin/overview => {online_devices,total_devices,today_messages,delivery_rate,avg_delivery_ms,total_users,total_classes,recent_messages:Message[],activity:[{date,count}]}
GET admin/activation-codes?search=&status= => ActivationCode[]
POST admin/activation-codes {count,max_teachers,max_devices,expires_at?,org_id?,note?} => ActivationCode[]
PATCH admin/activation-codes/:code {max_teachers?,max_devices?,expires_at?,status?} => ActivationCode
GET admin/users => User[]；POST admin/users {username,password,display_name,org_id?} => User
PATCH admin/users/:id {username?,password?,status?,display_name?} => User；DELETE admin/users/:id => {ok:true}（教师有负责班级时先转移或删除）
GET admin/classes => Class[]；POST admin/classes {name,grade?,owner_id,max_teachers,max_devices} => Class
PATCH admin/classes/:id {name?,grade?,owner_id?,max_teachers?,max_devices?} => Class；DELETE admin/classes/:id => {ok:true}
GET admin/devices => Device[]；GET admin/messages => {items:Message[],next_cursor:null}；GET admin/audit-logs => [{id,actor,action,target,at}]

## WebSocket
帧 {t,id?,seq?,ts?,d:{...}}。连接5秒内 hello {d:{token,last_seq?,client_version?,os?,pending_ids?}}。
welcome d:{class:{id,name},volume,server_time,recalled_ids?}; notify d:{message_id,style,text,student_name,signature,class_name,audio_url?,chime,repeat,display_sec,volume,sent_at,expires_at}; recall d:{message_id}; config d:{volume}; unbind d:{}；tool d:{name,params}; ping/pong；ack d:{message_id,status:delivered|playing|played|dismissed|failed}; receipt d:{message_id,class_id,device_id,status,at}; presence d:{class_id,device_id,online}。

## 前端共享包（主 Agent 提供）
@ classding/ui (实际包名 @classding/ui): 导出 cn, Button(variant default|secondary|ghost|outline|destructive, size default|sm|icon), Input, Textarea, Card, Badge(variant default|secondary|outline|success|warning|destructive), Dialog({open,onClose,title,description?,children,footer?}), Switch({checked,onCheckedChange,aria-label?}), NoticeScreen({text,studentName?,signature?,className?,style?,progress?,compact?,sentAt?}), Brand({compact?})。
共享 CSS 从 @classding/ui/styles.css 引入。所有组件支持 className。
@classding/api: api<T>(path,RequestInit?) 自动加当前 access token；setAccessToken(token),getAccessToken(),ApiError；types.ts 从 api/openapi.yaml 生成。api('/classes') 自动加 /api/v1。admin/app 可独立本地写结构类型等待生成，但最终须改为生成类型引用。

## 工作边界
主 Agent：仓库根配置、web/packages/ui、教师端、部署和 CI、整体验证。
后端 Agent：server/、api/、web/packages/api/（包括生成 types）、自己的 README。实现真实 SQLite/权限与最小端到端测试；Go 可下载安装到项目 tools/，不改系统。
管理端 Agent：只写 web/apps/admin/；绿色+暖白+精致简洁桌面后台，真实接口，禁止伪装数据。独立配置 Vite port 5174，proxy /api /ws 到 localhost:8080。基础路径 /admin/。
大屏 Agent：只写 screen/；Wails v2 Go 壳、绑定页、队列、WS重连、去重、本地语音/音效/倒计时；前端 port 5175，proxy 到8080；引用共享组件；Go Windows能力做真实可交叉编译实现或明确标注待完成，勿虚假占位承诺。

本地开发：Go :8080；教师 :5173；管理 :5174 (/admin/)；大屏 :5175。所有前端包 scripts dev/build/typecheck。共享 React Query Provider 等由每个应用自行装配。后端允许开发用种子开关及仅在开启时创建 demo 账号（teacher / ClassDing2026!，admin / AdminDing2026!），不得生产自动启用。无真实服务时显示错误/引导，不能假装已发送成功。

`pnpm api:generate` 从契约定义重新生成 OpenAPI、WS Schema 及两套 TypeScript 类型。WebSocket 业务类型从 `@classding/api/ws` 导入，接收原始 JSON 时仍要进行运行时检查。设备 `hello.pending_ids` 最多 4096 条；`welcome.recalled_ids` 用于确认离线队列中的撤回/过期消息。

浏览器请求携带 `X-ClassDing-Auth-Scope: admin|teacher`，对应独立的刷新 Cookie。`refreshSession()` 合并同一页面并发刷新；服务端提供 30 秒旋转宽限。管理员仅使用账号密码登录，动态验证码已移除。

POST admin/users/:id/unlock-login => {ok:true}，解除教师密码错误造成的短时限制；仅管理员有权操作同作用域教师，写入审计日志。User 可带 login_locked_until。
