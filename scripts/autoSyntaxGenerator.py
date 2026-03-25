# Python helper script to generate Go constants for repetitive bitboard masks.
def createBitboardMasks():
    value = sum(1 << (i * 8) for i in range(8))
    value1 = value << 1
    value2 = value << 2
    value3 = value << 3
    value4 = value << 4
    value5 = value << 5
    value6 = value << 6
    value7 = value << 7

    print("mask1:="+bin(value))
    print("mask2:="+bin(value1))
    print("mask3:="+bin(value2))
    print("mask4:="+bin(value3))
    print("mask5:="+bin(value4))
    print("mask6:="+bin(value5))
    print("mask7:="+bin(value6))
    print("mask8:="+bin(value7))
createBitboardMasks()