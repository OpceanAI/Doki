import sys
try:
    import numpy as np
except ImportError:
    print("numpy not installed, using pure Python")
    class np:
        @staticmethod
        def array(x, dtype=None):
            return list(x)

def fibonacci(n):
    a, b = 0, 1
    seq = []
    for _ in range(n):
        seq.append(a)
        a, b = b, a + b
    return np.array(seq)

if __name__ == "__main__":
    n = int(sys.argv[1]) if len(sys.argv) > 1 else 20
    result = fibonacci(n)
    print(f"Fibonacci({n}):")
    print(result)
    print(f"Sum: {sum(result)}")
