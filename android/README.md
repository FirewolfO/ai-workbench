# AI Workbench Android

原生 WebView 客户端，固定连接 `https://ai.lxvb.top`。Android 客户端只提供 People 企业身份登录，并从 `https://apps.lxvb.top/api/apps/ai-workbench/latest` 检查新版本。项目使用 API 36 编译并以 API 36 为目标，最低支持 API 35，即 Android 15 和 Android 16。

使用 JDK 17 和 Android SDK Platform 36 构建：

```bash
./gradlew assembleDebug
```

调试 APK 位于 `app/build/outputs/apk/debug/app-debug.apk`。客户端支持 Web 多附件选择、系统返回导航、下载、People OAuth、系统栏安全区和应用内版本更新。应用会在启动及回到前台时自动检查新版本，并在功能菜单底部常驻显示版本状态和手动检查入口。
