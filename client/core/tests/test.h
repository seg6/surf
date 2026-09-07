#ifndef SURF_TEST_H
#define SURF_TEST_H

#include <stdio.h>
#include <stdlib.h>

#define SURF_CHECK(condition)                                                  \
    do {                                                                       \
        if (!(condition)) {                                                    \
            fprintf(stderr, "%s:%d: check failed: %s\n", __FILE__, __LINE__, \
                    #condition);                                               \
            exit(EXIT_FAILURE);                                                \
        }                                                                      \
    } while (0)

#endif
