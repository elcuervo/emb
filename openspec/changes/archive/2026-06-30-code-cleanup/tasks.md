## 1. Remove mutex from client.rb

- [x] 1.1 Remove `@mutex = Mutex.new`, remove `require "mutex"` (if standalone), replace `@mutex.synchronize` blocks with direct assignment, change `pool` method to `@pool ||= default_pool`
