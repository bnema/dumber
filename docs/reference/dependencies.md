# Dependencies

This page lists the system dependencies required to run Dumber on different Linux distributions.

## Arch Linux

### Core GTK4 and WebKit

```bash
sudo pacman -S gtk4 webkitgtk-6.0
```

### VA-API (Hardware Video Acceleration)

```bash
sudo pacman -S libva libva-utils
```

#### VA-API Drivers (install one based on GPU)

**Intel (Broadwell and newer):**
```bash
sudo pacman -S intel-media-driver
```

**Intel (older):**
```bash
sudo pacman -S libva-intel-driver
```

**AMD:**
```bash
sudo pacman -S libva-mesa-driver
```

**NVIDIA:**
```bash
sudo pacman -S libva-nvidia-driver  # or nvidia-vaapi-driver from AUR
```

### Vulkan

```bash
sudo pacman -S vulkan-icd-loader
```

**Intel:**
```bash
sudo pacman -S vulkan-intel
```

**AMD:**
```bash
sudo pacman -S vulkan-radeon
```

**NVIDIA:**
```bash
sudo pacman -S nvidia-utils  # includes Vulkan
```

### GStreamer Plugins (for WebKit Media Playback)

```bash
sudo pacman -S gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly gst-libav
```

### Verification Tools

```bash
sudo pacman -S vulkan-tools  # for vulkaninfo
```

## Debian/Ubuntu

### Core GTK4 and WebKit

```bash
sudo apt install libgtk-4-1 libwebkitgtk-6.0-4
```

### VA-API

```bash
sudo apt install libva2 vainfo
```

#### VA-API Drivers

**Intel:**
```bash
sudo apt install intel-media-va-driver  # or i965-va-driver for older
```

**AMD:**
```bash
sudo apt install mesa-va-drivers
```

**NVIDIA:**
Requires manual setup or third-party PPA.

### Vulkan

```bash
sudo apt install libvulkan1 vulkan-tools
```

**Intel:**
```bash
sudo apt install mesa-vulkan-drivers
```

**AMD:**
```bash
sudo apt install mesa-vulkan-drivers
```

**NVIDIA:**
```bash
sudo apt install nvidia-vulkan-icd
```

### GStreamer

```bash
sudo apt install gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav
```

## Verification Commands

### Check VA-API

```bash
vainfo
```

### Check Vulkan

```bash
vulkaninfo | grep VK_KHR_video_
```

### Monitor GPU Decoder Usage

```bash
nvtop  # or intel_gpu_top for Intel
```

## Environment Variables

If auto-detection fails, you can force a specific VA-API driver:

```bash
# Intel (newer)
export LIBVA_DRIVER_NAME=iHD

# Intel (older)
export LIBVA_DRIVER_NAME=i965

# AMD
export LIBVA_DRIVER_NAME=radeonsi
```
