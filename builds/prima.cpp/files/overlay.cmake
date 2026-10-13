# Included at the END of the pinned source CMakeLists.txt by prepare.py.
set(NNG_TESTS OFF CACHE BOOL "" FORCE)
set(NNG_TOOLS OFF CACHE BOOL "" FORCE)
set(NNG_ENABLE_TLS OFF CACHE BOOL "" FORCE)
set(NNG_ENABLE_HTTP OFF CACHE BOOL "" FORCE)
# WebSocket implicitly enables SHA1 (FPL); the ring uses TCP only.
set(NNG_TRANSPORT_WS OFF CACHE BOOL "" FORCE)
set(NNG_TRANSPORT_WSS OFF CACHE BOOL "" FORCE)
set(NNG_SUPP_SHA1 OFF CACHE BOOL "" FORCE)
set(NNG_TRANSPORT_TCP ON CACHE BOOL "" FORCE)
add_subdirectory("${CMAKE_CURRENT_SOURCE_DIR}/deps/nng" nng)
# CI run 38468420048 (tag 0.1.2-6): the Windows leg (MinGW Makefiles +
# CLANG64 clang) reaches 45% then fails in the pinned nng dependency:
# src/platform/windows/win_tcpconn.c:263-264 passes SOCKET (UINT_PTR) to
# CancelIoEx(HANDLE, ...), and newer clang promotes -Wint-conversion to an
# error by default. Downgrade just the clang-default-error classes to
# warnings, for the nng target on Windows only; pinned source bytes stay
# untouched (prepare.py still asserts the nng SHA). Scan of the failure
# class: every other CancelIoEx/io-registration site in the pinned nng
# already casts to (HANDLE); the sibling classes
# (-Wincompatible-pointer-types, -Wimplicit-function-declaration) add no
# observed site in nng or the first-party ring files, and join this block
# preemptively so a latent Windows-only branch cannot burn another CI run
# one error at a time. C++ targets need no equivalent: these diagnostics
# are hard errors in C++, not downgradable warnings.
if(WIN32)
  target_compile_options(nng PRIVATE
    -Wno-error=int-conversion
    -Wno-error=incompatible-pointer-types
    -Wno-error=implicit-function-declaration)
endif()
target_include_directories(llama PRIVATE "${CMAKE_CURRENT_SOURCE_DIR}/common")
target_link_libraries(llama PUBLIC nng)
target_sources(llama PRIVATE src/network-utils.cpp)
target_compile_features(llama PUBLIC cxx_std_17)
target_sources(common PRIVATE common/profiler.cpp)
target_include_directories(common PRIVATE "${CMAKE_CURRENT_SOURCE_DIR}/ggml/src")
target_compile_features(common PUBLIC cxx_std_17)
add_executable(prima-ring-test common/zmq_nng_shim_test.cpp)
target_compile_features(prima-ring-test PRIVATE cxx_std_17)
target_compile_options(prima-ring-test PRIVATE -UNDEBUG)
target_link_libraries(prima-ring-test PRIVATE nng)
