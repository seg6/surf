cmake_minimum_required(VERSION 3.16)

project(surf_client_core VERSION 0.1.0 LANGUAGES C)

include(CTest)

option(SURF_CORE_WARNINGS_AS_ERRORS "Treat client-core warnings as errors" ON)
option(SURF_CORE_SANITIZE "Enable address and undefined-behavior sanitizers" OFF)

add_library(surf_client_core STATIC
    src/core.c
    src/diagnostics.c
    src/frame.c
    src/h264.c
    src/media.c
    src/session.c
)
add_library(Surf::ClientCore ALIAS surf_client_core)

target_include_directories(surf_client_core
    PUBLIC
        $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
)
set_target_properties(surf_client_core PROPERTIES
    C_STANDARD 99
    C_STANDARD_REQUIRED YES
    C_EXTENSIONS NO
)

if(CMAKE_C_COMPILER_ID MATCHES "Clang|GNU")
    target_compile_options(surf_client_core PRIVATE
        -Wall
        -Wextra
        -Wpedantic
        -Wconversion
        -Wshadow
    )
    if(SURF_CORE_WARNINGS_AS_ERRORS)
        target_compile_options(surf_client_core PRIVATE -Werror)
    endif()
    if(SURF_CORE_SANITIZE)
        target_compile_options(surf_client_core PRIVATE
            -fno-omit-frame-pointer
            -fsanitize=address,undefined
        )
        target_link_options(surf_client_core PRIVATE
            -fno-omit-frame-pointer
            -fsanitize=address,undefined
        )
    endif()
endif()

if(BUILD_TESTING)
    function(surf_core_test name source)
        add_executable(${name} ${source})
        target_link_libraries(${name} PRIVATE Surf::ClientCore)
        set_target_properties(${name} PROPERTIES
            C_STANDARD 99
            C_STANDARD_REQUIRED YES
            C_EXTENSIONS NO
        )
        if(CMAKE_C_COMPILER_ID MATCHES "Clang|GNU")
            target_compile_options(${name} PRIVATE -Wall -Wextra -Wpedantic)
            if(SURF_CORE_WARNINGS_AS_ERRORS)
                target_compile_options(${name} PRIVATE -Werror)
            endif()
            if(SURF_CORE_SANITIZE)
                target_compile_options(${name} PRIVATE
                    -fno-omit-frame-pointer
                    -fsanitize=address,undefined
                )
                target_link_options(${name} PRIVATE
                    -fno-omit-frame-pointer
                    -fsanitize=address,undefined
                )
            endif()
        endif()
        add_test(NAME ${name} COMMAND ${name})
    endfunction()

    surf_core_test(surf_frame_test tests/frame_test.c)
    surf_core_test(surf_diagnostics_test tests/diagnostics_test.c)
    surf_core_test(surf_h264_test tests/h264_test.c)
    surf_core_test(surf_media_test tests/media_test.c)
    surf_core_test(surf_core_test tests/core_test.c)
    surf_core_test(surf_session_test tests/session_test.c)
endif()
