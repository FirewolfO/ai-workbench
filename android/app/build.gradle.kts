plugins {
    id("com.android.application")
}

android {
    namespace = "shop.lxvb.aiworkbench"
    compileSdk = 36

    defaultConfig {
        applicationId = "shop.lxvb.aiworkbench"
        minSdk = 35
        targetSdk = 36
        versionCode = 7
        versionName = "1.1.5"
        buildConfigField("String", "APP_CENTER_URL", "\"https://apps.lxvb.top\"")
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    buildFeatures {
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}
