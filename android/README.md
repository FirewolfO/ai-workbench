# AI Workbench Android

原生 WebView 客户端，固定连接 `https://ai.lxvb.top`。项目使用 API 36 编译并以 API 36 为目标，最低支持 API 35，即 Android 15 和 Android 16。

使用 JDK 17 和 Android SDK Platform 36 构建：

```bash
./gradlew assembleDebug
```

调试 APK 位于 `app/build/outputs/apk/debug/app-debug.apk`。客户端支持 Web 附件选择、系统返回导航、下载和外部链接跳转。
