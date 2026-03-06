// 最优化的 Fibonacci 程序 - 迭代版本
// 最优化的 Fibonacci 程序 - 迭代版本
// 最优化的 Fibonacci 程序 - 迭代版本
// 最优化的 Fibonacci 程序 - 迭代版本
// 时间复杂度: O(n), 空间复杂度: O(1)
// 避免了递归的栈溢出风险和函数调用开销

pub fn fib(n: usize) u64 {
    if (n == 0) return 0;
    if (n == 1) return 1;

    var a: u64 = 0;
    var b: u64 = 1;
    var i: usize = 2;

    while (i <= n) : (i += 1) {
        const temp = a + b;
        a = b;
        b = temp;
    }

    return b;
}

pub fn main() !void {
    const test_cases = [_]usize{ 0, 1, 2, 3, 5, 10, 20, 30, 40, 50 };

    for (test_cases) |n| {
        const result = fib(n);
        const expected = switch (n) {
            0 => 0,
            1 => 1,
            2 => 1,
            3 => 2,
            5 => 5,
            10 => 55,
            20 => 6765,
            30 => 832040,
            40 => 102334155,
            50 => 12586269025,
            else => unreachable,
        };
        
        if (result == expected) {
            std.debug.print("fib({d}) = {d} ✓\n", .{ n, result });
        } else {
            std.debug.print("fib({d}) = {d}, expected {d} ✗\n", .{ n, result, expected });
        }
    }

    // 性能测试：计算较大的 Fibonacci 数
    std.debug.print("\n性能测试:\n", .{});
    const large_n = 1000;
    const large_result = fib(large_n);
    std.debug.print("fib({d}) = {d}\n", .{ large_n, large_result });

    // 内存占用测试（通过编译时间优化选项）
    // 使用 -O3 编译可获得最佳性能
    std.debug.print("\n建议使用: zig build -O3 ReleaseFast\n", .{});
}

test "fibonacci basic" {
    const tests = std.testing.expectEqualArrays(
        [_]u64{0, 1, 1, 2, 3, 5, 8, 13, 21, 34},
        [_]u64{
            fib(0),
            fib(1),
            fib(2),
            fib(3),
            fib(4),
            fib(5),
            fib(6),
            fib(7),
            fib(8),
            fib(9),
        },
    );
    try tests;
}

test "fibonacci large" {
    // 测试较大的 n 值
    const result = fib(1000);
    // 验证结果不为 0 且为有效值
    if (result == 0) {
        try std.testing.fail("fib(1000) should not be 0");
    }
}

// 避免了递归的栈溢出风险和函数调用开销

pub fn fib(n: usize) u64 {
    if (n == 0) return 0;
    if (n == 1) return 1;

    var a: u64 = 0;
    var b: u64 = 1;
    var i: usize = 2;

    while (i <= n) : (i += 1) {
        const temp = a + b;
        a = b;
        b = temp;
    }

    return b;
}

pub fn main() !void {
    const test_cases = [_]usize{ 0, 1, 2, 3, 5, 10, 20, 30, 40, 50 };

    for (test_cases) |n| {
        const result = fib(n);
        const expected = switch (n) {
            0 => 0,
            1 => 1,
            2 => 1,
            3 => 2,
            5 => 5,
            10 => 55,
            20 => 6765,
            30 => 832040,
            40 => 102334155,
            50 => 12586269025,
            else => unreachable,
        };
        
        if (result == expected) {
            std.debug.print("fib({d}) = {d} ✓\n", .{ n, result });
        } else {
            std.debug.print("fib({d}) = {d}, expected {d} ✗\n", .{ n, result, expected });
        }
    }

    // 性能测试：计算较大的 Fibonacci 数
    std.debug.print("\n性能测试:\n", .{});
    const large_n = 1000;
    const large_result = fib(large_n);
    std.debug.print("fib({d}) = {d}\n", .{ large_n, large_result });

    // 内存占用测试（通过编译时间优化选项）
    // 使用 -O3 编译可获得最佳性能
    std.debug.print("\n建议使用: zig build -O3 ReleaseFast\n", .{});
}

test "fibonacci basic" {
    const tests = std.testing.expectEqualArrays(
        [_]u64{0, 1, 1, 2, 3, 5, 8, 13, 21, 34},
        [_]u64{
            fib(0),
            fib(1),
            fib(2),
            fib(3),
            fib(4),
            fib(5),
            fib(6),
            fib(7),
            fib(8),
            fib(9),
        },
    );
    try tests;
}

test "fibonacci large" {
    // 测试较大的 n 值
    const result = fib(1000);
    // 验证结果不为 0 且为有效值
    if (result == 0) {
        try std.testing.fail("fib(1000) should not be 0");
    }
}

// 避免了递归的栈溢出风险和函数调用开销

pub fn fib(n: usize) u64 {
    if (n == 0) return 0;
    if (n == 1) return 1;

    var a: u64 = 0;
    var b: u64 = 1;
    var i: usize = 2;

    while (i <= n) : (i += 1) {
        const temp = a + b;
        a = b;
        b = temp;
    }

    return b;
}

pub fn main() !void {
    const test_cases = [_]usize{ 0, 1, 2, 3, 5, 10, 20, 30, 40, 50 };

    for (test_cases) |n| {
        const result = fib(n);
        const expected = switch (n) {
            0 => 0,
            1 => 1,
            2 => 1,
            3 => 2,
            5 => 5,
            10 => 55,
            20 => 6765,
            30 => 832040,
            40 => 102334155,
            50 => 12586269025,
            else => unreachable,
        };
        
        if (result == expected) {
            std.debug.print("fib({d}) = {d} ✓\n", .{ n, result });
        } else {
            std.debug.print("fib({d}) = {d}, expected {d} ✗\n", .{ n, result, expected });
        }
    }

    // 性能测试：计算较大的 Fibonacci 数
    std.debug.print("\n性能测试:\n", .{});
    const large_n = 1000;
    const large_result = fib(large_n);
    std.debug.print("fib({d}) = {d}\n", .{ large_n, large_result });

    // 内存占用测试（通过编译时间优化选项）
    // 使用 -O3 编译可获得最佳性能
    std.debug.print("\n建议使用: zig build -O3 ReleaseFast\n", .{});
}

test "fibonacci basic" {
    const tests = std.testing.expectEqualArrays(
        [_]u64{0, 1, 1, 2, 3, 5, 8, 13, 21, 34},
        [_]u64{
            fib(0),
            fib(1),
            fib(2),
            fib(3),
            fib(4),
            fib(5),
            fib(6),
            fib(7),
            fib(8),
            fib(9),
        },
    );
    try tests;
}

test "fibonacci large" {
    // 测试较大的 n 值
    const result = fib(1000);
    // 验证结果不为 0 且为有效值
    if (result == 0) {
        try std.testing.fail("fib(1000) should not be 0");
    }
}

// 避免了递归的栈溢出风险和函数调用开销

pub fn fib(n: usize) u64 {
    if (n == 0) return 0;
    if (n == 1) return 1;

    var a: u64 = 0;
    var b: u64 = 1;
    var i: usize = 2;

    while (i <= n) : (i += 1) {
        const temp = a + b;
        a = b;
        b = temp;
    }

    return b;
}

pub fn main() !void {
    const test_cases = [_]usize{ 0, 1, 2, 3, 5, 10, 20, 30, 40, 50 };

    for (test_cases) |n| {
        const result = fib(n);
        const expected = switch (n) {
            0 => 0,
            1 => 1,
            2 => 1,
            3 => 2,
            5 => 5,
            10 => 55,
            20 => 6765,
            30 => 832040,
            40 => 102334155,
            50 => 12586269025,
            else => unreachable,
        };
        
        if (result == expected) {
            std.debug.print("fib({d}) = {d} ✓\n", .{ n, result });
        } else {
            std.debug.print("fib({d}) = {d}, expected {d} ✗\n", .{ n, result, expected });
        }
    }

    // 性能测试：计算较大的 Fibonacci 数
    std.debug.print("\n性能测试:\n", .{});
    const large_n = 1000;
    const large_result = fib(large_n);
    std.debug.print("fib({d}) = {d}\n", .{ large_n, large_result });

    // 内存占用测试（通过编译时间优化选项）
    // 使用 -O3 编译可获得最佳性能
    std.debug.print("\n建议使用: zig build -O3 ReleaseFast\n", .{});
}

test "fibonacci basic" {
    const tests = std.testing.expectEqualArrays(
        [_]u64{0, 1, 1, 2, 3, 5, 8, 13, 21, 34},
        [_]u64{
            fib(0),
            fib(1),
            fib(2),
            fib(3),
            fib(4),
            fib(5),
            fib(6),
            fib(7),
            fib(8),
            fib(9),
        },
    );
    try tests;
}

test "fibonacci large" {
    // 测试较大的 n 值
    const result = fib(1000);
    // 验证结果不为 0 且为有效值
    if (result == 0) {
        try std.testing.fail("fib(1000) should not be 0");
    }
}

