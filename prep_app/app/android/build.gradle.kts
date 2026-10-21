allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

val newBuildDir: Directory =
    rootProject.layout.buildDirectory
        .dir("../../build")
        .get()
rootProject.layout.buildDirectory.value(newBuildDir)

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)
}
// file_picker 8.3.7 фиксирует compileSdk 34, а flutter_plugin_android_lifecycle
// требует API 36. targetSdk и minSdk приложения не трогаем.
subprojects {
    afterEvaluate {
        val android = extensions.findByName("android") ?: return@afterEvaluate
        val cls = android.javaClass
        val getter = cls.methods.firstOrNull { it.name == "getCompileSdk" && it.parameterCount == 0 }
        val setter = cls.methods.firstOrNull {
            it.name == "setCompileSdk" &&
                it.parameterCount == 1 &&
                (it.parameterTypes[0] == Int::class.javaPrimitiveType ||
                    it.parameterTypes[0] == Integer::class.java)
        }
        if (getter != null && setter != null) {
            val current = (getter.invoke(android) as? Number)?.toInt() ?: 0
            if (current < 36) {
                setter.invoke(android, 36)
            }
        } else {
            val legacy = cls.methods.firstOrNull {
                it.name == "compileSdkVersion" &&
                    it.parameterCount == 1 &&
                    it.parameterTypes[0] == Int::class.javaPrimitiveType
            }
            legacy?.invoke(android, 36)
        }
    }
}
subprojects {
    project.evaluationDependsOn(":app")
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
