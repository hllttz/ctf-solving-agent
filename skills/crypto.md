Use Sage when algebraic or number-theoretic structure appears.

First checks:
- Parse parameters and identify whether the problem is RSA, lattice, finite field, elliptic curve, polynomial, or encoding based.
- Use Python with pycryptodome/gmpy2/sympy for simple arithmetic and format handling.
- Prefer sage for finite fields, elliptic curves, polynomial rings, LLL, and Coppersmith-style small-root work.

Useful commands:
- sage -v
- sage solve.sage
- sage -c 'print(GF(101)(3)^5)'
- cat > /workspace/solve.sage <<'EOF'
  from sage.all import *
  print(GF(101)(3)^5)
  EOF
  sage /workspace/solve.sage

Flag discipline:
- Do not report a decrypted value until it decodes to a plausible printable flag or the challenge output verifies it.
