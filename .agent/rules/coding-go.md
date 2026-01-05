---
trigger: model_decision
description: Go 编码规则
---

* 错误判断优先用 errors.Is
* 用 any 替代 interface{}
* Go 中没有进程/线程的说法，只有协程