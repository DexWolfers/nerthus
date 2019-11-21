newdisk="/dev/vdb" # disk path
mountdir="/datadisk" # dictionary

(
echo o # Create a new empty DOS partition table
echo n # Add a new partition
echo p # Primary partition
echo 1 # Partition number
echo   # First sector (Accept default: 1)
echo   # Last sector (Accept default: varies)
echo w # Write changes
) | fdisk $newdisk

partprobe
mkfs -t ext4 $newdisk
mkdir -p $mountdir
mount $newdisk $mountdir
df -TH